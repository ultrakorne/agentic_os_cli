# Scheduler Engine — Technical

## Architecture

The engine lives in the Electron main process but **delegates the actual ticking to system cron**. On `engine.start()` it (1) loads the agent config store + runs store, (2) installs `wrapper.sh` from `resources/` into `<userData>/data/`, (3) ensures `<userData>/data/agents/` exists and seeds it on first run, (4) scans the agents directory tree for scripts, (5) checks for `crontab` and `python3` on PATH, (6) reconciles the managed block of the user's crontab, (7) starts a directory watcher on `runs/`, and (8) starts a 5-minute setInterval that re-runs missed-run detection.

Mutations all go through `engine.setSchedule(agentId, spec | null)` (which writes `agents.json` and rewrites the managed crontab block) or `engine.runManually(agentId)` (which spawns the wrapper directly with `AGENTIC_OS_TRIGGER=manual` and a pre-generated run id). The runs directory watcher converts wrapper writes into `scheduler:changed` IPC broadcasts so renderer windows re-fetch.

## Source Files

| File | Role |
|------|------|
| `resources/wrapper.sh` | Bash run-wrapper invoked by cron and by manual runs; writes `<run-id>.json` + `<run-id>.out`; reinstalled on every app start |
| `resources/agents/{ping.sh,disk-free.sh,README.md}` | Seed agents copied into `<userData>/data/agents/` on first run |
| `src/shared/scheduler.ts` | Public types shared across main/preload/renderer (`Agent`, `AgentConfig`, `JobRun`, `MissedRun`, `CrontabStatus`, `ScheduleSpec`) |
| `src/main/scheduler/types.ts` | Re-exports shared types for the main-process import path |
| `src/main/scheduler/spec.ts` | `compileToCron` + default spec constants; pure functions |
| `src/main/scheduler/spec.test.ts` | Spec compilation + previous-tick math tests |
| `src/main/scheduler/crontab.ts` | Read/write the user crontab; extract and rebuild the managed BEGIN/END block; matched-pair-only purge for reconciliation |
| `src/main/scheduler/crontab.test.ts` | Marker round-trip + block-build + purge tests |
| `src/main/scheduler/missed.ts` | Walk-forward expected-tick enumeration via croner's `nextRun`; tolerance match against actual runs |
| `src/main/scheduler/missed.test.ts` | Pure-function tests for missed-run detection |
| `src/main/scheduler/agent-config-store.ts` | Persists `<dataDir>/agents.json`; atomic-rename writes, single write queue; `setSchedule(id, spec | null)` is the main mutation point |
| `src/main/scheduler/engine.ts` | Joins config + scripts into `Agent[]`; manages crontab + missed sweep + manual-run spawn |
| `src/main/scheduler/runs-store.ts` | Reader for `runs/<id>.{json,out}`; fs.watch + debounce; lazy output read |
| `src/main/agents/scanner.ts` | Walks `<userData>/data/agents/` (top-level + first-level subdirs); section = parent folder; deduplicates ids |
| `src/main/agents/scanner.test.ts` | Extension policy + section-from-folder tests |
| `src/main/ipc.ts` | Wires engine + theme to `ipcMain.handle`; defines IPC channel constants |
| `src/main/index.ts` | App boot: builds stores, constructs the engine |

## Data Model

The on-disk shapes are documented under [data-layout](../data-layout/TECHNICAL.md). The summary:

- `agents.json` — agent config: id + optional schedule + optional title/description
- `runs/<run-id>.json` — meta record per run, written by `wrapper.sh`
- `runs/<run-id>.out` — captured stdout+stderr
- `wrapper.sh` — refreshed on every app start from `resources/wrapper.sh`
- `agents/<id>.<ext>` or `agents/<Section>/<id>.<ext>` — user-owned executable scripts

## Cron Layout

The managed section sits in the user's crontab between fixed markers:

```
# BEGIN agentic_os (managed - do not edit)
0 9 * * 1,2,3,4,5 '/…/wrapper.sh' '/…/data' 'agent-id' 'agent-id' '/…/agents/Daily/agent-id.sh' # agentic_os:agent-id
# END agentic_os
```

Each cron line carries a trailing `# agentic_os:<agentId>` tag. The tag lets the engine recover the agent id even if BEGIN/END markers are damaged, and makes hand-inspection easy. All four wrapper arguments are single-quoted (with proper `'\''` escaping) so paths with spaces or apostrophes survive cron parsing.

## Noteworthy Behavior

- **`previousRun()` is not used.** croner's `previousRun()` only returns a value if the cron has actually fired in-process — it doesn't compute "what would the prior tick be". `missed.detectMissed` walks forward via `cron.nextRun(cursor)` from `now - windowMs` to `now`, which is reliable.
- **Tolerance is 90s.** Generous to cover cron jitter, machine wake-up lag, and clock skew. A schedule for `:00` matches a run at `:00:30` or `:00:00.500`.
- **A later run clears prior missed slots.** `detectMissed` flags an expected tick T as missed only if the agent's most recent run started before `T - tolerance`. A manual run from the UI (or a catch-up scheduled run) implicitly acknowledges all prior gaps for that agent — the banner clears without waiting for the next on-time tick.
- **Missed-run sweep runs even when no UI is open.** A setInterval on the engine, fires every 5 minutes, broadcasts only when the missed set changes. The runs-directory watcher also triggers a sweep on every change so manual-run completions clear the banner immediately.
- **fs.watch is debounced 250ms.** When the wrapper writes the running meta and then the final meta, both events would otherwise re-broadcast. The 5-minute sweep also re-indexes the dir as a safety net.
- **Output is read lazily.** `RunsStore.list()` returns each run with a tail summary (last 4KB) populated at ingest time; full output is only loaded on `scheduler:read-run-output(runId)`.
- **Runs dir GC at engine start.** Files are grouped by run-id stem so `<id>.json` + `<id>.out` are always deleted as a pair, never leaving an orphan on either side.
- **Section comes from the script's parent directory.** `agents.json` never stores section. Top-level scripts get section `"Agents"`; first-level subdirectory names become section names. Deeper nesting is ignored. Duplicate ids across subdirectories are dropped (top-level wins, then alphabetical) with a console warning.
- **Orphan agents are *flagged*, not removed.** `engine.listAgents()` joins config entries against discovered scripts and sets `orphaned: true` on configs without a matching script. Orphans are excluded from crontab sync and rendered in red in the UI.
- **Wrapper is reinstalled on every start.** Cheap, and keeps the on-disk wrapper in lockstep with whatever ships in the asar.
- **Manual runs are detached and unrefed.** `child_process.spawn(wrapper, …, { detached: true, stdio: 'ignore' })` then `cp.unref()` — the run survives even if the user quits the app immediately after clicking. The run id is pre-generated and threaded as the wrapper's 5th argv, so the IPC stub's `id` matches what the wrapper writes to disk.
- **Cron PATH is minimal.** The wrapper exports `PATH=/usr/local/bin:/opt/homebrew/bin:/usr/bin:/bin:$PATH`. Anything past that is the script's responsibility.
- **Wrapper exposes context as env vars.** Before invoking the agent script, the wrapper exports `AGENTIC_OS_DATA_DIR` (the data root), `AGENTIC_OS_AGENT_ID`, `AGENTIC_OS_AGENT_SCRIPT`, `AGENTIC_OS_RUN_ID`, and `AGENTIC_OS_TRIGGER` (`schedule` or `manual`). User scripts use these to write portable paths — e.g. `"$AGENTIC_OS_DATA_DIR/workspaces/my_agent"` resolves correctly on Linux (`~/.config/agentic-os/data/...`) and macOS (`~/Library/Application Support/agentic-os/data/...`) without per-platform branches.
- **Crontab tampering is detected, not silently overwritten.** `extractManaged` flags duplicate / dangling markers as a conflict; the engine refuses to rewrite until the user invokes `scheduler:reconcile-crontab` (with a two-click confirm in the UI).
- **`purgeAllManaged` only removes complete BEGIN..END pairs.** A stray BEGIN with no END (or stray END) deletes only the marker line itself — never trailing user content.
- **Crontab read-modify-write is not atomic.** `crontab -l` followed by `crontab -` has a TOCTOU window. POSIX provides no compare-and-swap primitive. Mitigation: writes are infrequent (only on schedule edits), and the trailing `# agentic_os:<agentId>` per-line tags let a damaged managed section still be parsed back to original ids.
- **`crontab` and `python3` absence are surfaced, not hidden.** `engine.start()` checks both with `--version`; `crontabStatus` returns `crontabOk` / `pythonOk` / `wrapperOk` flags; the dashboard's `SystemBanner` renders an actionable error if anything's missing.
- **Cron daemon liveness is checked separately from the binary.** A user can have `crontab` on PATH but the daemon disabled (Arch ships `cronie.service` disabled by default — schedules are silently never fired). `crontabStatus.daemonOk` is computed by `pgrep -x` against `crond` / `cron` / `cronie`; the banner surfaces `daemonOk === false` with a `systemctl enable --now cronie` hint. `daemonOk` is `null` (silent) when detection isn't possible (Windows, missing `pgrep`).

## Dependencies

- `croner` — used for `compileToCron` source and `nextRun()` in `missed.detectMissed` and `engine.nextRunFor`
- `child_process` (Node) — `crontab -l`, `crontab -`, `python3 --version`, manual-run wrapper spawn
- `fs.watch` (Node) — runs directory observer
- `python3` (system) — required at runtime to JSON-encode wrapper meta files
- `crontab` (system) — required at runtime to read/write the user crontab (Linux: `cronie` or similar; macOS ships it)
- `electron` (main only) — `app.getPath('userData')`, `ipcMain`, `shell.openPath`
