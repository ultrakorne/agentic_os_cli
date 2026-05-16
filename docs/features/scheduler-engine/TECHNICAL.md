# Scheduler Engine — Technical

## Architecture

System cron is the ticker; the [`aos` CLI](../../../agentic_os_cli/) owns everything that touches the user's system (the agents tree, sidecar writes, the managed crontab block, the runs directory — including missed-slot records — and `wrapper.sh`); the Electron main process is a view that caches what the CLI emits and shells out for every mutation.

On `AppService.start()` the main process (1) loads the runs watcher, (2) shells out to `aos list --json` to populate the agent cache, (3) starts the watcher on `runs/`, then (4) fires `aos refresh --json` once to reconcile cron on boot. Schedule and description edits go through `aos schedule` / `aos describe`, both of which write the meta sidecar **and** reconcile cron in-process before printing the result. Manual "run now" shells out to `aos run <id> --json`, which spawns `wrapper.sh` detached and prints the `JobRun` stub.

## Source Files

| File | Role |
|------|------|
| `src/shared/scheduler.ts` | Public types shared across main/preload/renderer (`Agent`, `AgentMeta`, `JobRun`, `JobRunStatus`, `ScheduleSpec`, `RefreshSummary`). `JobRunStatus` includes `'missed'` — missed scheduled slots are persisted as `JobRun` records by the CLI rather than tracked in a separate store |
| `src/main/scheduler/types.ts` | Re-exports shared types for the main-process import path |
| `src/main/scheduler/runs-store.ts` | Reader for `runs/<id>.{json,out}`; fs.watch + debounce; lazy output read. Missed slots arrive here as `JobRun{status:"missed"}` entries written by `aos tick` / `aos refresh` |
| `src/main/agents/agent-list.ts` | Parses `aos list --json` into the renderer's `Agent[]`; converts `ScheduleSpec` back into CLI flag form for `aos schedule` |
| `src/main/agents/agent-list.test.ts` | Round-trip tests for the parser and the schedule-to-flags converter |
| `src/main/service.ts` | `AppService`: in-memory agent cache, shells out to `aos list/schedule/describe/refresh/run` for every mutation (including manual runs) |
| `src/main/ipc.ts` | Wires service + theme to `ipcMain.handle`; defines IPC channel constants |
| `src/main/index.ts` | App boot: builds stores, constructs the service |

## How this feature uses `aos`

The main process is a shell-out client. Every mutation goes through the CLI:

| Renderer action | CLI invocation | Notes |
|----------------|----------------|-------|
| App boot, focus, periodic refresh | `aos list --json` | Fills the agent cache |
| Boot, top-bar "refresh", SystemBanner reconcile | `aos refresh --json` | Re-scan + cron reconcile; returns the `RefreshSummary` |
| Schedule editor save / clear | `aos schedule <id> ...` (or `--off`) `--json` | CLI writes the sidecar **and** runs refresh internally; the embedded `refresh` field in the JSON output is plucked into `lastRefresh` so no second spawn is needed |
| Description editor save / clear | `aos describe <id> "<text>" --json` | Sidecar-only write; no refresh (descriptions don't affect cron) |
| Manual run | `aos run <id> --json` | CLI mints the run id, spawns `wrapper.sh` detached with `AGENTIC_OS_TRIGGER=manual`, and prints the `JobRun` stub. Keeps the cron and manual invocation forms of the wrapper in one place so they can't drift |

The renderer still reads runs through the in-process `RunsStore` (fs.watch + 4KB output tail caching for the dashboard's live preview). The same data is available from the terminal via `aos runs [--agent <id>] [--limit N] [--json]` for listing and `aos runs <run-id> [--json|--output]` for single-run records or raw .out dumps — useful for scripting and debugging without opening the UI.

What the CLI actually does — its verbs, flags, JSON output shapes, and the sidecar-write rules — is documented inside the CLI repo (see [`agentic_os_cli/docs/`](../../../agentic_os_cli/docs/) and its [`README.md`](../../../agentic_os_cli/README.md)). This document only covers how the Electron app calls into it.

## Data Model

The on-disk shapes are documented under [data-layout](../data-layout/TECHNICAL.md). The summary:

- `agents/<id>.<ext>` or `agents/<Section>/<id>.<ext>` — user-owned executable scripts
- `agents/<id>.meta.json` — optional per-agent sidecar: schedule + scheduledAt + optional title/description
- `runs/<run-id>.json` — meta record per run, written by `wrapper.sh`
- `runs/<run-id>.out` — captured stdout+stderr
- `wrapper.sh` — refreshed on every app start from `resources/wrapper.sh`

## Cron Layout

The managed section sits in the user's crontab between fixed markers:

```
# BEGIN agentic_os (managed - do not edit)
0 9 * * 1,2,3,4,5 '/…/wrapper.sh' '/…/data' 'agent-id' 'agent-id' '/…/agents/Daily/agent-id.sh' # agentic_os:agent-id
# END agentic_os
```

Each cron line carries a trailing `# agentic_os:<agentId>` tag. The tag lets the engine recover the agent id even if BEGIN/END markers are damaged, and makes hand-inspection easy. All four wrapper arguments are single-quoted (with proper `'\''` escaping) so paths with spaces or apostrophes survive cron parsing.

## Noteworthy Behavior

- **Missed-slot detection lives in the CLI, not the renderer.** `aos tick` (invoked by cron every 10 minutes) and `aos refresh` walk each agent's cron expression forward to find the most-recent uncovered expected slot ≤ now, and persist it as a `JobRun{status:"missed"}` into `<aos_home>/runs/`. The renderer never recomputes — it reads the runs directory.
- **Only the latest miss per agent is recorded.** When a newer uncovered slot is detected, the previous miss file for that agent is deleted and replaced; at most one `miss-*` record per agent exists on disk at any time. The deliberate granularity loss (multi-slot outages collapse to one entry) is what lets the dashboard show "agents currently behind" as a one-row-per-agent banner that auto-resolves on the next real run.
- **A later run clears the "behind" banner.** The dashboard derives its missed-runs banner by taking the latest run per agent and filtering to `status === 'missed'`. A successful manual or scheduled run becomes the new latest for that agent and the banner clears immediately; the historical miss record stays in the run history.
- **fs.watch is debounced 250ms.** When the wrapper writes the running meta and then the final meta, both events would otherwise re-broadcast. The cron-driven `aos tick` (every 10 minutes) also writes/replaces miss records when applicable, which doubles as a safety net for any watch events the OS drops.
- **Output is read lazily.** `RunsStore.list()` returns each run with a tail summary (last 4KB) populated at ingest time; full output is only loaded on `scheduler:read-run-output(runId)`.
- **Runs dir GC at engine start.** Files are grouped by run-id stem so `<id>.json` + `<id>.out` are always deleted as a pair, never leaving an orphan on either side.
- **Section comes from the script's parent directory.** Sidecars never store section. Top-level scripts get section `"Agents"`; first-level subdirectory names become section names. Deeper nesting is ignored. Duplicate ids across subdirectories are dropped (top-level wins, then alphabetical) with a console warning.
- **A sidecar with no matching script is silently ignored.** Lone `*.meta.json` files don't appear in `agents:list`. If the user later adds the matching script, the sidecar attaches automatically. There is no orphan banner.
- **Non-executable scripts are surfaced, not dropped silently.** A file with a supported shell extension (or no extension and a `#!` shebang) sitting under `agents/` but missing the `+x` bit is collected by `findScanIssues` as `{ kind: 'not-executable', path }`. The dashboard's `SystemBanner` renders a per-file warn row so the user knows why their script didn't appear. Reserved-id and duplicate-id rejections still only log to the main-process console (low signal-to-noise for end users; the warning suffices).
- **Empty meta = no file.** If a write would produce `{}` (e.g. clearing a description on an unscheduled agent), the sidecar is `unlink`ed instead of left as a stub.
- **Wrapper is reinstalled on every start.** Cheap, and keeps the on-disk wrapper in lockstep with whatever ships in the asar.
- **Manual runs are detached and survive app exit.** `aos run <id>` starts `wrapper.sh` in a new session (`setsid`) so the run keeps going even if the user quits the app immediately after clicking. The CLI pre-generates the run id and threads it as the wrapper's 5th argv, so the stub returned to the renderer matches the file the wrapper writes to disk.
- **Cron PATH is minimal.** The wrapper exports `PATH=$HOME/.local/bin:$HOME/bin:/usr/local/bin:/opt/homebrew/bin:/usr/bin:/bin:$PATH`. The `~/.local/bin` entry is what lets `claude` (and other npm-style per-user installs) resolve from a cron-launched run. Anything past those dirs is the script's responsibility.
- **Failing scripts with empty output get a wrapper-level hint.** If `EC != 0` and the `.out` file is zero bytes, the wrapper writes a one-paragraph note pointing at the most common cause (`OUTPUT=$(cmd)` swallowing stdout when the script exits before replaying it — e.g. `claude --print` writing its 401 error to stdout, never to .out). Keeps the dashboard's expanded error row from being blank.
- **Wrapper exposes context as env vars.** Before invoking the agent script, the wrapper exports `AGENTIC_OS_DATA_DIR` (the data root), `AGENTIC_OS_AGENT_ID`, `AGENTIC_OS_AGENT_SCRIPT`, `AGENTIC_OS_RUN_ID`, and `AGENTIC_OS_TRIGGER` (`schedule` or `manual`). User scripts use these to write portable paths — e.g. `"$AGENTIC_OS_DATA_DIR/workspaces/my_agent"` resolves correctly on Linux (`~/.config/agentic-os/data/...`) and macOS (`~/Library/Application Support/agentic-os/data/...`) without per-platform branches.
- **Crontab tampering is detected, not silently overwritten.** `extractManaged` flags duplicate / dangling markers as a conflict; the engine refuses to rewrite until the user invokes `scheduler:reconcile-crontab` (with a two-click confirm in the UI).
- **`purgeAllManaged` only removes complete BEGIN..END pairs.** A stray BEGIN with no END (or stray END) deletes only the marker line itself — never trailing user content.
- **Crontab read-modify-write is not atomic.** `crontab -l` followed by `crontab -` has a TOCTOU window. POSIX provides no compare-and-swap primitive. Mitigation: writes are infrequent (only on schedule edits), and the trailing `# agentic_os:<agentId>` per-line tags let a damaged managed section still be parsed back to original ids.
- **`crontab` and `python3` absence are surfaced, not hidden.** `engine.start()` checks both with `--version`; `crontabStatus` returns `crontabOk` / `pythonOk` / `wrapperOk` flags; the dashboard's `SystemBanner` renders an actionable error if anything's missing.
- **Cron daemon liveness is checked separately from the binary.** A user can have `crontab` on PATH but the daemon disabled (Arch ships `cronie.service` disabled by default — schedules are silently never fired). `crontabStatus.daemonOk` is computed by `pgrep -x` against `crond` / `cron` / `cronie`; the banner surfaces `daemonOk === false` with a `systemctl enable --now cronie` hint. `daemonOk` is `null` (silent) when detection isn't possible (Windows, missing `pgrep`).

## Dependencies

- `child_process` (Node) — spawning the `aos` CLI for every read and mutation (`list`, `schedule`, `describe`, `refresh`, `run`)
- `fs.watch` (Node) — runs directory observer
- `python3` (system) — required at runtime to JSON-encode wrapper meta files
- `crontab` (system) — required at runtime to read/write the user crontab (Linux: `cronie` or similar; macOS ships it)
- `electron` (main only) — `app.getPath('userData')`, `ipcMain`, `shell.openPath`
