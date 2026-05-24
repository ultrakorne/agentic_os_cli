# scheduler — Technical

## Architecture

Two verbs (`cmd/aos/refresh.go`, `cmd/aos/tick.go`) sit on top of a shared `internal/scheduler/` package plus the platform-native backend in `internal/scheduler/backend/`. The scheduler package owns scanning, schedule validation, miss detection, miss persistence, and the stale-running sweep. The backend package owns the per-platform file format, atomic write, drift comparison, and the `launchctl` / `systemctl --user` orchestration. `internal/scheduler/schedspec/` is the leaf package holding `ScheduleSpec` + `Weekday` — extracted so both the scheduler core and the backend implementations can import it without an import cycle. Runs sweep and log trim live in their own small packages (`internal/scheduler` for the runs sweep, `internal/logtrim`) and are wired in by refresh.

## Source Files

| File | Role |
|------|------|
| `cmd/aos/refresh.go` | `aos refresh` verb: orchestrates scan → miss record → `backend.Sync` → log trim → runs sweep; `runRefresh` is called in-process by `init` and `schedule` |
| `cmd/aos/tick.go` | `aos tick` verb: prints the `LogLine` form (or `--json`) the on-disk tick.log receives |
| `internal/scheduler/tick.go` | `Tick` body: scan → record missed → sweep stale running → probe `backend.State` → append tick.log; `TickOutcome` type |
| `internal/scheduler/refresh.go` | `Refresh` body: scan → record misses → call `backend.Sync` → trim log → sweep runs; `RefreshOutcome` + `BackendSyncOutcome` types |
| `internal/scheduler/spec.go` | Type aliases that re-export `schedspec.ScheduleSpec` / `Weekday` for callers; `ValidateSchedule` (rejects invalid specs before persistence) |
| `internal/scheduler/schedspec/spec.go` | Leaf package: `ScheduleSpec`, `Weekday`, `NextSlot` iterator (the native replacement for `robfig/cron/v3`'s slot walk) |
| `internal/scheduler/missed.go` | `DetectMissed`: walk `ScheduleSpec.NextSlot` forward from `scheduledAt`, find the latest uncovered slot per agent |
| `internal/scheduler/missed_record.go` | `RecordMissedRuns`: atomic write of the latest miss, replacing any older one for the agent; returns post-write runs slice |
| `internal/scheduler/missed_record_test.go` | Miss persistence: replacement, no-op-when-current-slot-already-recorded, error surfacing |
| `internal/scheduler/stale.go` | `SweepStaleRunning`: rewrites `running` records older than `StaleRunningThreshold` (1 h) as `error: "no completion record"` |
| `internal/scheduler/spawn.go` | `SpawnWrapperDetached` (manual runs only); `NewRunID` |
| `internal/scheduler/linger_linux.go` | `probeLinger`: shells out to `loginctl show-user --property=Linger`, returns `ok | disabled | unknown` |
| `internal/scheduler/linger_other.go` | macOS/other-platform shim: always returns "" so refresh skips the LingerState field |
| `internal/scheduler/backend/backend.go` | `Backend` interface (`Sync`, `Remove`, `State`); `Spec`, `AgentJob`, `TickJob`, `SyncResult`, `FailedJob`, `State` constants; `Select(aosHome)` factory |
| `internal/scheduler/backend/launchd_darwin.go` | macOS LaunchAgent backend: renders `launchdJob` → plist via `howett.net/plist`, atomic-writes, bootout/bootstrap-cycles when content changed; orphan namespace cleanup |
| `internal/scheduler/backend/launchd_loader_darwin.go` | `realLaunchdLoader`: thin shell-out wrapper for `launchctl bootstrap|bootout|print gui/$UID/<label>` |
| `internal/scheduler/backend/systemd_linux.go` | Linux systemd-user backend: renders INI `.service`+`.timer` pairs, atomic-writes, `enable --now` / `disable --now`; orphan cleanup with single `daemon-reload` per Sync |
| `internal/scheduler/backend/systemd_loader_linux.go` | `realSystemdLoader`: shell-out wrapper for `systemctl --user enable|disable|is-active|daemon-reload` |
| `internal/scheduler/backend/select_{darwin,linux,other}.go` | Build-tagged factory: returns the platform backend, or nil on unsupported GOOS (caller hard-errors) |
| `cmd/aos/init_linger_linux.go` | Interactive prompt that offers `sudo loginctl enable-linger $USER` after `aos init` when linger is off on a headless host (`XDG_SESSION_TYPE` empty or `"tty"`) |
| `cmd/aos/init_linger_other.go` | macOS/other-platform shim: prompt no-op |
| `internal/config/config.go` | `EffectiveTickInterval`: parses `tick_interval` (Go duration); `DefaultTickInterval = "1h"` |
| `internal/runtime/runtime.go` | `HasBin`, `IsExecutable`, `FileExists`, `AosBinaryPath` (resolved absolute path baked into the tick job's argv) |
| `internal/logtrim/logtrim.go` | `Trim`: head-trim a file to `keepBytes` when it exceeds `maxBytes` (256 KiB / 128 KiB defaults); preserves partial-line safety |
| `internal/scheduler/run_store.go` | `FileRunStore.Sweep`: groups `<id>.{json,out}` pairs, drops oldest by mtime when count exceeds cap |
| `internal/resources/wrapper.sh` | The wrapper invoked by the backend job; new argv is `<data-dir> <agent-id> <script-path> [<run-id>]`; traps SIGTERM/SIGINT to write an `interrupted by reload` error record |

## Data Model

### `TickOutcome` (JSON contract for `aos tick`)

```go
type TickOutcome struct {
    Timestamp     string   `json:"timestamp"`
    Scripts       int      `json:"scripts"`
    Scheduled     int      `json:"scheduled"`
    Missed        int      `json:"missed"`         // newly written this tick
    StaleResolved int      `json:"staleResolved"`  // running→error rewrites this tick
    Backend       string   `json:"backend"`        // managed | drift | empty | error(<msg>)
    Warnings      []string `json:"warnings,omitempty"`
}
```

The `[tick] ...` line in `tick.log` is rendered from `TickOutcome.LogLine()`:

```
[tick] 2026-05-24T13:00:00Z scripts=2 scheduled=1 missed=0 staleResolved=0 backend=managed
```

### `RefreshOutcome` (JSON contract for `aos refresh`)

```go
type RefreshOutcome struct {
    Agents        int                `json:"agents"`
    Scheduled     int                `json:"scheduled"`
    Issues        int                `json:"issues"`
    Backend       BackendSyncOutcome `json:"backend"`
    Wrapper       HealthState        `json:"wrapper"`
    Python3       HealthState        `json:"python3"`
    BackendHealth HealthState        `json:"backendHealth"`
    LingerState   HealthState        `json:"lingerState,omitempty"` // Linux only
    Log           LogTrimOutcome     `json:"log"`
    Runs          RunsSweepOutcome   `json:"runs"`
    Warnings      []string           `json:"warnings,omitempty"`
}

type BackendSyncOutcome struct {
    State     string   `json:"state"`              // managed | drift | empty | skipped
    Reasons   []string `json:"reasons,omitempty"`  // skip reasons (no-wrapper, no-python3, …)
    Wrote     int      `json:"wrote"`
    Unchanged int      `json:"unchanged"`
    Removed   int      `json:"removed"`
    Failed    []string `json:"failed,omitempty"`   // "<agent-id>: <reason>"
}
```

`HealthState` is one of `"ok" | "missing" | "down" | "unknown" | "disabled"`.

### Backend file formats

#### macOS (`~/Library/LaunchAgents/com.agenticos.<id>.plist`)

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.agenticos.ping</string>
    <key>ProgramArguments</key>
    <array>
        <string>/home/you/.aos/wrapper.sh</string>
        <string>/home/you/.aos</string>
        <string>ping</string>
        <string>/home/you/.aos/agents/ping.sh</string>
        <string></string>
    </array>
    <key>StartCalendarInterval</key>
    <array>
        <dict>
            <key>Minute</key><integer>0</integer>
            <key>Hour</key><integer>23</integer>
            <key>Weekday</key><integer>1</integer>
        </dict>
        ...
    </array>
    <key>RunAtLoad</key>
    <false/>
    <key>StandardOutPath</key>
    <string>/dev/null</string>
    <key>StandardErrorPath</key>
    <string>/dev/null</string>
    <key>EnvironmentVariables</key>
    <dict>
        <key>AGENTIC_OS_TRIGGER</key>
        <string>schedule</string>
    </dict>
</dict>
</plist>
```

`StartCalendarInterval` is an array of dicts — one per `(weekday, hour, minute)` tuple. `Minute`, `Hour`, and `Weekday` are encoded as pointer fields in the Go struct so omitting any of them produces launchd's "match every value" semantics. The 5th `ProgramArguments` element is the empty run-id slot — wrapper.sh mints its own id when this is empty.

#### Linux (`~/.config/systemd/user/agentic-os-<id>.{service,timer}`)

```ini
[Unit]
Description=aos agent ping

[Service]
Type=oneshot
ExecStart='/home/you/.aos/wrapper.sh' '/home/you/.aos' 'ping' '/home/you/.aos/agents/ping.sh' ''
Environment=AGENTIC_OS_TRIGGER=schedule
StandardOutput=null
StandardError=null
```

```ini
[Unit]
Description=aos agent ping timer

[Timer]
OnCalendar=Mon,Tue,Wed,Thu,Fri *-*-* 23:00:00
Persistent=true
Unit=agentic-os-ping.service

[Install]
WantedBy=timers.target
```

The tick job is the same shape with `OnBootSec=10s` + `OnUnitActiveSec=<tick_interval>s` instead of `OnCalendar`, and `StandardOutput=append:<tick.log>` so the periodic tick's stdout lands in the log.

### Wrapper argv contract

```
wrapper.sh <aos-home> <agent-id> <script-path> [<run-id>]
```

The 4th arg is optional: scheduled invocations leave it empty so the wrapper mints `<unix-ms>-<rand4>` itself; manual runs from `aos run` pass an explicit id so the in-process stub matches the on-disk record. The previous cron-era 5-arg form `<aos-home> <schedule-id> <agent-id> <script-path> [run-id]` is gone — there is no `scheduleId` concept anymore.

## Noteworthy Behavior

- **`NextSlot` is the native replacement for `robfig/cron/v3`.** `ScheduleSpec.NextSlot(after)` evaluates the spec in `after.Location()` and returns the earliest scheduled instant strictly after `after`. Hourly walks up to 48 hours forward; daily walks up to 8 days. Returns the zero time for invalid specs. `DetectMissed` calls it in a loop to find the latest slot ≤ now.
- **Miss coverage uses three rules and only emits the latest uncovered slot.** `isCovered` accepts a slot if (a) any run matches it within `±jitter` (default 30 s) regardless of status — including a previously-recorded miss, which acts as an ack; (b) any terminal run (`success`/`error`) at-or-after the slot; (c) any `running` record at-or-after `slot - jitter` (wrapper in flight). The walk caps at `lookbackBound = 8 days` and `maxTicks = 500` so the slowest schedule we support (weekly daily) always terminates quickly.
- **Drift comparison is structural, not byte-equal.** `launchdPlistEqual` parses the on-disk plist via `howett.net/plist` and `reflect.DeepEqual`s the resulting `launchdJob` against the freshly rendered one; this tolerates plist whitespace and key-order variations. `unitsEqualOnDisk` parses the systemd INI into the same slice-of-sections form used to render and compares with `reflect.DeepEqual`. Comments and blank lines are dropped in both directions so they're ignored by the comparator.
- **Sync writes are atomic via temp + rename.** Both backends write to `<path>.tmp` and `os.Rename` on top of the target. A crashed Sync never leaves a half-written plist or unit; a concurrent reader sees either the old or the new file.
- **Orphan cleanup is unconditional.** Any file in the namespace (`com.agenticos.*.plist` / `agentic-os-*.{service,timer}`) that isn't in the freshly rendered expected set is booted-out / disabled and unlinked. There's no human-edits-preserved fallback: each agent owns its own file, so there's nothing for a user to legitimately leave behind in the namespace.
- **`launchctl bootout` errors are swallowed when the job isn't loaded.** Loader matches `"could not find"` / `"no such process"` in the output and treats them as success — re-running Sync against a partially-installed namespace shouldn't fail on stale labels.
- **`systemctl --user enable --now` is idempotent and called on every Sync.** Re-enabling an already-enabled unit is a no-op; the explicit call is cheap insurance against a unit getting `disable`d out-of-band. `daemon-reload` is called once before disables/enables only when something actually changed on disk.
- **Stale-running sweep mutates in place via atomic write.** `rewriteStaleRunning` reads the run record, rewrites `Status`, `EndedAt`, `Error`, and `ExitCode`, and writes via `<path>.tmp` + `os.Rename`. Threshold is `time.Hour`; the rewrite message is `"no completion record"` and the synthetic `exitCode` is `1`.
- **Wrapper traps SIGTERM/SIGINT.** When launchd's `bootout` or systemd's `disable --now` lands during a wrapper run, the wrapper's signal trap fires `write_meta error "$END" 143 "interrupted by reload"; exit 143` so no orphaned `running` record is left for the next stale-running sweep to handle.
- **Lex-comparison on `startedAt` strings is a trap.** `wrapper.sh` writes `.123Z`, `aos run` writes `.000Z`, but `time.RFC3339Nano` on a zero-subsecond miss strips the fraction (`Z`). `.` (46) < `Z` (90), so same-second mixed-format records lex-invert. Both `ReadRuns` and the sweep compare `StartedAtTime` (parsed `time.Time`) instead.
- **Refresh reuses `RecordMissedRuns`'s post-write runs slice.** Tick passes that slice into `SweepStaleRunning` so a 2000-entry `runs/` isn't walked twice per tick.
- **A warned agent (e.g. `not-executable`) is excluded from the backend spec but counted in `issues`.** Without this, the backend would install a job that can't run; the operator wouldn't know why nothing happened. The count surfaces in the refresh summary.
- **`backend.Sync` reports per-agent failures inline, doesn't short-circuit.** Each failing agent is appended to `SyncResult.Failed` with a reason string; the sweep continues with the rest. The refresh summary surfaces the failures as `"<agent-id>: <reason>"` strings under `backend.failed`.
- **`tick.interval` parse failures fall back and continue.** Both refresh and tick log a warning (`warn: invalid tick_interval ...`) and use `DefaultTickInterval` (`1h`) for this round. The next refresh after the user fixes the config rewrites the tick job's interval.
- **`Trim` head-trims with line-boundary awareness.** The file's last `keepBytes` are kept, but the kept window starts at the first newline within it so partial lines are dropped. Without this, the head of a log entry could lose its prefix and confuse tail consumers.
- **`Sweep` groups by stem.** A run owns up to two files (`<id>.json` and `<id>.out`); deleting one without the other would leave an orphan. The sweeper groups by stem, sorts by max mtime across the pair, and deletes both files atomically.
- **`AosBinaryPath` resolves symlinks.** The tick job's argv uses the resolved absolute path so a re-install to a different directory requires a refresh to update the entry — and so a symlink swap doesn't silently change what the backend runs.
- **Linger probe is Linux-only.** `probeLinger` shells out to `loginctl show-user $USER --property=Linger --value`; `"yes"` → `ok`, `"no"` → `disabled`, anything else → `unknown`. The macOS shim always returns the empty string so refresh skips `LingerState` in the output. The `init` interactive prompt only fires on headless sessions (`XDG_SESSION_TYPE` empty or `"tty"`) and only at a TTY; under `--json` it's skipped entirely.

## Dependencies

- `internal/scheduler` — scanner, schedule validation, miss detection, stale sweep, spawn (manual runs only).
- `internal/scheduler/backend` — platform-native scheduler (`launchd_darwin.go` + `systemd_linux.go` behind `Backend` interface).
- `internal/scheduler/schedspec` — leaf package holding `ScheduleSpec` + `Weekday`; imported by both `scheduler` and `backend`.
- `internal/config` — `aos_home`, `runs_hard_cap`, `tick_interval`.
- `internal/runtime` — health probes (`HasBin`, `IsExecutable`, `AosBinaryPath`).
- `internal/logtrim` — log trim wired in by refresh.
- `howett.net/plist` — macOS plist encode/decode.
