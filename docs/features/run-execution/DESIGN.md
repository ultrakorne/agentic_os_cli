# run-execution — Design

## Overview

The "spawn a wrapper and look at the result" surface. `aos run` fires a manual run by spawning `wrapper.sh` detached from the CLI process — by default the verb prints a Run stub and returns immediately, so the operator (or a script) can record the new run id without waiting; with `--wait`, the verb also blocks until the wrapper finishes and prints the captured output. `aos runs` reads the wrapper-written `runs/` directory and renders either a recent-runs table or one run's record with its `.out` inline.

## Components

### `aos run <id>` (default)

Looks up the agent by id, estimates duration from up to 10 newest **successful** runs (error/running/missed runs are skipped so a fast-failing script doesn't pull the ETA down), mints a run id, spawns `wrapper.sh` detached (`setsid`) with `AGENTIC_OS_TRIGGER=manual` and the explicit run id as argv[5], then prints a Run stub on stdout and exits. The wrapper writes the terminal record (`success` / `error`) under `<aos_home>/runs/<run-id>.json` once the script exits; the operator polls or watches for that file.

### `aos run <id> --wait`

Same stub-first behavior, then blocks until the wrapper writes a terminal record. While waiting, a Bubble Tea progress bar (when an estimate exists) or spinner (no history) renders on **stderr** so the run summary on stdout stays untouched. After the wrapper finishes, the raw bytes of `<run-id>.out` are appended to stdout. `Ctrl+C` while waiting prints a one-line message to stderr and exits non-zero — the detached wrapper keeps running.

### `aos runs` (list)

Reads every `<run-id>.json` under `<aos_home>/runs/`, drops malformed records, optionally filters by `--agent <id>`, sorts by `startedAt` descending, caps at `--limit` (default 25). Human output is a styled table with status-colored rows; JSON output is `{"runs": [...]}`. A muted `showing N of M runs` line precedes the table when `--limit` is hiding records.

### `aos runs <run-id>` (single)

Reads one run record and its sibling `<run-id>.out`, renders a key/value block plus an `output` section with the captured stdout/stderr. Under `--json` the inner record (no `runs` wrapper) is emitted with `output` populated from the `.out` file. Missed runs render with no `output` section because they have no `.out` file.

## User Flows

### Fire-and-forget manual run

1. User runs `aos run my-agent`.
2. CLI looks up the agent, mints a run id, spawns `wrapper.sh` detached, prints the stub, exits.
3. Wrapper exec'd in a new session writes `running` immediately, runs the script, writes `success` or `error` on exit.
4. Operator polls `aos runs <run-id>` (or watches the file) to see the result.

### Wait for the result

1. User runs `aos run my-agent --wait`.
2. CLI prints the stub on stdout.
3. Progress bar (or spinner) renders on stderr while polling the run record.
4. Once the wrapper writes the terminal record, the bar collapses, the `.out` bytes are written to stdout, and the verb exits with the run's exit code (non-zero if the agent failed).
5. `Ctrl+C` during the wait prints "run is still executing in the background" on stderr and exits non-zero; the agent keeps running.

### Browse recent runs

1. User runs `aos runs` (or `aos runs --agent my-agent --limit 50`).
2. The styled table shows recent runs newest-first. `STATUS` is colored amber (running), green (success), red (error), yellow (missed). `ELAPSED` is `...` while running and `—` for missed runs.
3. To dive in: `aos runs <run-id>` shows the full record plus the captured `.out` as an `output` section.

## Design Decisions

- **Stub-first is the contract.** Even with `--wait`, the stub prints immediately after the wrapper is spawned — so an operator using `--json --wait` sees the stub JSON first, then the `.out` bytes appended. Scripted consumers that want only structured output drop `--wait`.
- **Progress goes on stderr.** The wait progress bar/spinner is rendered with `tea.WithOutput(os.Stderr)` so piping stdout into another tool isn't disturbed. The final layout is always `stdout: stub → stderr: progress → stdout: .out`.
- **Run id is minted up-front and threaded through the wrapper.** The wrapper's argv[5] carries the engine-minted run id so the stub's `id` field matches the file the wrapper will write — no second poll to discover the wrapper's chosen id.
- **Estimate is a 10-sample average.** `EstimateRunDuration` averages elapsed time across the newest 10 completed runs. Fewer samples = less reliable; no samples ⇒ `-1` (JSON) / `"none"` (human), and the wait UI shows an indeterminate spinner instead of a fake progress bar.
- **`Ctrl+C` is "stop watching," not "stop running."** The wrapper was spawned detached (`setsid`), so SIGINT to the CLI doesn't propagate. The wait flow exits non-zero with a clear message so shells/scripts can distinguish "I gave up waiting" from "the run finished."
- **A failed run prints `.out` first, then exits non-zero.** Stderr from the script is captured into `.out` together with stdout, so showing it before exiting preserves the operator's view of *why* the run failed. The exit code carries the underlying status.
- **Run records are written atomically.** The wrapper writes `<run-id>.json.tmp` and `mv`s on top. `LoadRuns` and `ReadRun` skip malformed files silently because a concurrent reader can still hit a partial state in rare cases.
- **`aos runs` list omits `output`.** Listing N runs with full transcripts could balloon to megabytes — the list view leaves `output` empty and the single-run view populates it from `.out`. Operators wanting a raw transcript use `aos runs <id> --json | jq -r .output`.
