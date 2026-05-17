# run-execution — Technical

## Architecture

`aos run` orchestrates: look up the agent (scanner), estimate duration (`EstimateRunDuration`), mint a run id (`NewRunID`), spawn `wrapper.sh` detached (`SpawnWrapperDetached`), and either print-and-return or hand off to the wait flow. The wait flow runs a Bubble Tea program on stderr that polls `<run-id>.json` until the wrapper writes a terminal status. `aos runs` only reads — list and single-run views share `ReadRuns` / `ReadRun` / `ReadRunOutput`. The wrapper itself owns the start/end/exit/output capture contract — see `internal/resources/wrapper.sh`.

## Source Files

| File | Role |
|------|------|
| `cmd/aos/run.go` | `aos run` verb: agent lookup, estimate, spawn, stub print, optional wait dispatch |
| `cmd/aos/run_test.go` | `aos run` happy-path + estimate tests |
| `cmd/aos/runs.go` | `aos runs` list + single-run views; status coloring; "showing N of M" hint |
| `cmd/aos/wait.go` | `aos run --wait` flow: Bubble Tea program on stderr, polling, final `.out` print |
| `cmd/aos/wait_test.go` | Wait-flow tests including cancel + non-zero exit propagation |
| `internal/scheduler/runs.go` | `Run` type; `LoadRuns`, `ReadRuns`, `ReadRun`, `ReadRunOutput`, `EstimateRunDuration` |
| `internal/scheduler/runs_test.go` | Run-record read/filter/sort tests |
| `internal/scheduler/spawn.go` | `SpawnOpts`, `SpawnWrapperDetached` (uses `Setsid`), `NewRunID` |
| `internal/scheduler/spawn_test.go` | Spawn smoke tests |
| `internal/scheduler/wait.go` | `WaitForRun` polling loop with `ErrWaitCanceled` for context-cancel |
| `internal/scheduler/wait_test.go` | Wait loop tests |
| `internal/resources/wrapper.sh` | The shell wrapper executed by cron and by `aos run` (start/end/exit/output capture) |

## Data Model

### `Run` (on disk: `<run-id>.json`)

```go
type Run struct {
    ID         string    `json:"id"`
    AgentID    string    `json:"agentId"`
    ScheduleID *string   `json:"scheduleId"`  // for catch-ups: missed-slot timestamp
    Trigger    string    `json:"trigger"`     // "schedule" | "manual" | "catch-up"
    StartedAt  string    `json:"startedAt"`   // RFC3339 with millisecond precision
    EndedAt    *string   `json:"endedAt"`     // null while running / missed
    Status     RunStatus `json:"status"`      // "running" | "success" | "error" | "missed"
    Output     string    `json:"output"`      // empty in list views; populated in single-run view from .out
    Error      *string   `json:"error"`
    ExitCode   *int      `json:"exitCode"`
    OutputPath *string   `json:"outputPath"`  // typically "<run-id>.out"
}
```

Optional fields use pointers so JSON round-trips as `null` instead of dropping or zero-defaulting.

### Wrapper argv contract

```
wrapper.sh <aos-home> <schedule-id|''> <agent-id> <script-path> <run-id>
```

Trigger is conveyed via the `AGENTIC_OS_TRIGGER` env var (defaults to `"schedule"` inside the wrapper). The wrapper also exports `AGENTIC_OS_DATA_DIR`, `AGENTIC_OS_AGENT_ID`, `AGENTIC_OS_AGENT_SCRIPT`, `AGENTIC_OS_RUN_ID`, and `AGENTIC_OS_TRIGGER` for the agent script.

## Noteworthy Behavior

- **Run ids are minted ahead of the spawn.** `NewRunID` produces `<unix-ms>-<rand4>`. Threading it as wrapper argv[5] (and into `AGENTIC_OS_RUN_ID`) is how the stub `aos run` prints matches the on-disk file. Without an explicit id the wrapper would mint its own and the operator would have no way to correlate.
- **`SpawnWrapperDetached` uses `Setsid`.** The wrapper runs in a new session so SIGINT to the CLI doesn't propagate; the agent's lifetime is decoupled from the operator's shell. `cmd.Process.Release()` releases OS-level resources after `Start`.
- **`ReadRuns` sorts by `StartedAtTime`, not the raw string.** Writers emit subseconds inconsistently — wrapper.sh carries ms (`.123Z`), `aos run` forces 3-digit ms (`.000Z`), but `time.RFC3339Nano` on a zero-subsecond miss strips the fraction entirely (`Z`). ASCII `.` (46) < `Z` (90), so a same-second mixed-format collision would lex-invert. Parsing into `time.Time` and comparing avoids the trap.
- **`EstimateRunDuration` averages the newest 10 completed runs.** Runs without a parseable `endedAt` are ignored; if none qualify, the function returns `(0, false, nil)` and callers print `"none"` / emit `-1`. The estimate is rounded to 100 ms in human output for clean display.
- **Wait polls every 250 ms by default.** `WaitForRun` treats `NotFoundError` as "still running" (the wrapper writes atomically, but the file doesn't exist for the first few hundred ms after spawn). Any other read/parse error surfaces immediately.
- **`Ctrl+C` during wait sets `canceled=true` on the model.** `waitFlow` checks the flag, prints "wait canceled — run … is still executing in the background" on stderr, and returns `ErrWaitCanceled`. The detached wrapper is unaffected.
- **`aos runs` list view filters out malformed records silently.** A record missing `id`, `agentId`, or `startedAt` is skipped. The wrapper writes atomically via temp+rename, but concurrent reads against a record being written can still hit a partial state in rare cases; silent skip is preferred over a noisy diagnostic that would race.
- **`STATUS` colors carry the pass/fail signal at list scale.** The single-run view shows the exit code; the list view doesn't, because the color already conveys the result. `ELAPSED` is `...` while running and `—` for missed (never ran).
- **`output` is empty in list payloads.** A list of 50 runs with full transcripts could be megabytes — list keeps it empty and tells operators "use single-run for the transcript."
- **Wrapper writes a fallback error message when a failing script produces no output.** Scripts that capture stdout via `$(...)` and exit before replaying it leave the user with an empty `.out`; the wrapper detects this (non-zero exit, empty file) and writes a short diagnostic suggesting `$(cmd 2>&1)`. Cosmetic but high-leverage in practice.
- **macOS quarantine xattr is stripped before exec.** Scripts that arrived via download/scp/AirDrop carry `com.apple.quarantine`, which Gatekeeper rejects with "bad interpreter: Operation not permitted." The wrapper strips it on Darwin only (Linux has no `xattr` and the call no-ops).

## Dependencies

- `internal/scheduler` — `FindAgentByID`, `SpawnWrapperDetached`, `NewRunID`, `EstimateRunDuration`, `ReadRuns`, `ReadRun`, `ReadRunOutput`, `WaitForRun`.
- `internal/config` — for `aos_home`.
- `internal/resources/wrapper.sh` — the executed wrapper.
- `charm.land/bubbletea/v2`, `charm.land/bubbles/v2/progress`, `charm.land/bubbles/v2/spinner` — wait UI.
