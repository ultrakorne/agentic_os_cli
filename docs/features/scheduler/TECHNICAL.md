# scheduler — Technical

## Architecture

Two verbs (`cmd/aos/refresh.go`, `cmd/aos/tick.go`) sit on top of a shared `internal/scheduler/` package plus the cron block writer in `internal/crontab/`. The scheduler package owns scanning, schedule compilation, miss detection, miss persistence, and catch-up candidate selection. The crontab package owns the bracket-marker block, atomic crontab read/write, and a process-local lock. Runs sweep and log trim live in their own small packages (`internal/runsgc`, `internal/logtrim`) and are wired in by refresh.

## Source Files

| File | Role |
|------|------|
| `cmd/aos/refresh.go` | `aos refresh` verb: orchestrates scan → entries → cron sync → miss record → runs sweep → log trim; `RunRefresh` is called in-process by `init` and `schedule` |
| `cmd/aos/tick.go` | `aos tick` verb: scan → record misses → fire catch-ups → log line; `crontabState` reports `managed | empty | drift | conflict | error(...)` |
| `cmd/aos/tick_test.go` | Tick integration tests (drift detection, catch-up gate, log append) |
| `internal/scheduler/spec.go` | `ScheduleSpec`, `AgentMeta`, `CompileToCron` (hourly + daily → crontab(5) expression) |
| `internal/scheduler/missed.go` | `DetectMissed`: walk cron schedule, find latest uncovered slot per agent (rules a/b/c documented in source) |
| `internal/scheduler/missed_record.go` | `RecordMissedRuns`: atomic write of the latest miss, replacing any older one for the agent; returns post-write runs slice |
| `internal/scheduler/missed_record_test.go` | Miss persistence: replacement, no-op-when-current-slot-already-recorded, error surfacing |
| `internal/scheduler/catchup.go` | `DetectCatchups`: "latest run is missed" gate; stable ordering by agent id |
| `internal/scheduler/catchup_test.go` | Catch-up gate tests (success/error/running short-circuits, missing schedule skip) |
| `internal/scheduler/spawn.go` | `SpawnWrapperDetached` (used for both manual and catch-up runs); `NewRunID` |
| `internal/crontab/crontab.go` | `ReadCrontab` / `WriteCrontab` (shell out to `crontab -l` / `crontab -`), `ExtractManaged`, `BuildManagedBlock`, `SyncCrontab`, `RemoveManaged` |
| `internal/crontab/crontab_test.go` | Marker extraction + block sync tests |
| `internal/crontab/lock.go` | File-lock under `<aos_home>/.crontab.lock` with 10s timeout, 30s stale threshold |
| `internal/config/config.go` | `EffectiveTickCronExpr`: parses `tick_interval` (Go duration) into `*/N * * * *` or `0 */H * * *` |
| `internal/runtime/runtime.go` | `HasBin`, `IsExecutable`, `FileExists`, `AosBinaryPath` (resolved absolute path baked into cron), `CronDaemonRunning` (pgrep across `crond`/`cron`/`cronie`) |
| `internal/logtrim/logtrim.go` | `Trim`: head-trim a file to `keepBytes` when it exceeds `maxBytes` (256 KiB / 128 KiB defaults); preserves partial-line safety |
| `internal/runsgc/runsgc.go` | `Sweep`: groups `<id>.{json,out}` pairs, drops oldest by mtime when count exceeds cap |

## Data Model

### `TickSummary`

```go
type TickSummary struct {
    Timestamp string `json:"timestamp"`
    Scripts   int    `json:"scripts"`
    Scheduled int    `json:"scheduled"`
    Missed    int    `json:"missed"`   // newly written this tick (not outstanding)
    Catchups  int    `json:"catchups"` // wrappers actually spawned this tick
    Crontab   string `json:"crontab"`  // managed | empty | conflict | drift | error(<msg>)
}
```

### `RefreshSummary`

```go
type RefreshSummary struct {
    Agents    int    `json:"agents"`
    Scheduled int    `json:"scheduled"`
    Issues    int    `json:"issues"`
    Cron      string `json:"cron"`    // wrote | unchanged | skipped:<reason>
    Wrapper   string `json:"wrapper"` // ok | missing
    Python3   string `json:"python3"` // ok | missing
    Daemon    string `json:"daemon"`  // ok | down | unknown
    Log       string `json:"log"`     // trimmed | untouched
    Runs      string `json:"runs"`    // untouched | swept:<n> | skipped:<reason>
}
```

### Managed block format

```
# BEGIN agentic_os (managed - do not edit)
<tick-schedule> <aos-bin> tick >> <aos-home>/tick.log 2>&1 # agentic_os:__tick__
<entry-schedule> '<wrapper>' '<aos-home>' '<agent-id>' '<agent-id>' '<script-path>' # agentic_os:<agent-id>
...
# END agentic_os
```

Every value is shell-quoted (`'...'` with embedded quotes escaped). The trailing `# agentic_os:<id>` comment is the per-line marker used to attribute a line to a specific agent.

## Noteworthy Behavior

- **Miss coverage uses three rules and only emits the latest uncovered slot.** `isCovered` accepts a slot if (a) any run matches it within `±jitter` (default 30s) regardless of status — including a previously-recorded miss, which acts as an ack; (b) any terminal run (`success`/`error`) at-or-after the slot; (c) any `running` record at-or-after `slot - jitter` (wrapper in flight). The walk caps at `lookbackBound = 8 days` and `maxTicks = 500` so the slowest schedule we support (weekly daily) always terminates quickly.
- **Lex-comparison on `startedAt` strings is a trap.** `wrapper.sh` writes `.123Z`, `aos run` writes `.000Z`, but `time.RFC3339Nano` on a zero-subsecond miss strips the fraction (`Z`). `.` (46) < `Z` (90), so same-second mixed-format records lex-invert. Both `ReadRuns` and `DetectCatchups` compare `StartedAtTime` (parsed `time.Time`) instead.
- **Refresh reuses `RecordMissedRuns`'s post-write runs slice.** Tick passes that slice into `fireCatchups` so a 2000-entry `runs/` isn't walked twice per tick.
- **Catch-up failures don't loop.** `DetectCatchups`'s gate is strictly "latest is missed." A failed catch-up writes `status: "error"`, which becomes the latest run and stops further auto-fires for that agent. The operator surfaces the error via the dashboard or `aos runs --agent <id>`.
- **A warned agent (e.g. `not-executable`) is excluded from cron but counted in `issues`.** Without this, cron would fire a script that can't run; the operator wouldn't know why nothing happened. The count surfaces in the refresh summary.
- **Cron sync is lock-protected.** `acquireLock` opens `<aos_home>/.crontab.lock` exclusive; a lock older than 30 s is considered stale and removed. The wait timeout is 10 s — a contended lock returns `(SyncResult{Reason: "crontab lock contended"}, nil)` so the verb reports `skipped:...` rather than aborting.
- **The cron block is only rewritten when content differs.** `computeNext` produces the full new crontab text; if it equals `current`, nothing is written and `cron=unchanged` is reported. This is how refresh stays idempotent.
- **Damaged/duplicated markers report `conflict` and are left alone.** A user who hand-edited the block into a broken state gets a `cron=skipped:conflict` result rather than having their changes purged. `SyncArgs.Force` (currently unused) would override.
- **`tick_interval` parse failures fall back, log, and continue.** Both refresh and tick log a warning on stderr (`warn: invalid tick_interval ...`) and use `DefaultTickInterval` for this round. The next refresh after the user fixes the config will rewrite the cron line.
- **`Trim` head-trims with line-boundary awareness.** The file's last `keepBytes` are kept, but the kept window starts at the first newline within it so partial lines are dropped. Without this, the head of a log entry could lose its prefix and confuse tail consumers.
- **`Sweep` groups by stem.** A run owns up to two files (`<id>.json` and `<id>.out`); deleting one without the other would leave an orphan. The sweeper groups by stem, sorts by max mtime across the pair, and deletes both files atomically.
- **`crontabState` (tick-only) reports drift.** Even without a refresh in this call, tick rebuilds the *expected* block and compares it to the on-disk one — if they differ, the summary says `drift`. The dashboard shows this so the operator knows to run refresh.
- **`AosBinaryPath` resolves symlinks.** The managed `__tick__` line uses the resolved absolute path so a re-install to a different directory requires a refresh to update cron — and so a symlink swap doesn't silently change what cron runs.

## Dependencies

- `internal/scheduler` — scanner, schedule compilation, miss detection, catch-up gate, spawn.
- `internal/crontab` — atomic crontab read/write, managed-block format, file lock.
- `internal/config` — `aos_home`, `runs_hard_cap`, `catchup_enabled`, `tick_interval`.
- `internal/runtime` — health probes (`HasBin`, `IsExecutable`, `CronDaemonRunning`, `AosBinaryPath`).
- `internal/runsgc`, `internal/logtrim` — runs sweep and log trim wired in by refresh.
- `github.com/robfig/cron/v3` — cron expression parser used by `DetectMissed`.
