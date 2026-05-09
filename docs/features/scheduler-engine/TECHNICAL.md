# Scheduler Engine — Technical

## Architecture

The engine lives in the Electron main process but **delegates the actual ticking to system cron**. On `engine.start()` it (1) loads stores, (2) installs `wrapper.sh` from `resources/` into `<userData>/data/`, (3) ensures `<userData>/agents/` exists and seeds it on first run, (4) scans the agents directory for `AgentMeta`, (5) checks for `crontab` and `python3` on PATH, (6) reconciles the managed block of the user's crontab, (7) starts a directory watcher on `runs/`, and (8) starts a 5-minute setInterval that re-runs missed-run detection.

Mutations (`upsertSchedule`, `removeSchedule`, manual `runNow`) all go through the engine. Schedule mutations rewrite the managed crontab block. Manual runs spawn the wrapper directly with `AGENTIC_OS_TRIGGER=manual`. The runs directory watcher converts wrapper writes into `scheduler:changed` IPC broadcasts so renderer windows re-fetch.

## Source Files

| File | Role |
|------|------|
| `resources/wrapper.sh` | Bash run-wrapper invoked by cron and by manual runs; writes `<run-id>.json` + `<run-id>.out`; reinstalled on every app start |
| `resources/agents/{ping.sh,disk-free.sh,README.md}` | Seed agents copied into `<userData>/agents/` on first run |
| `src/shared/scheduler.ts` | Public types shared across main/preload/renderer (`Schedule`, `JobRun`, `Agent`, `MissedRun`, `CrontabStatus`) |
| `src/main/scheduler/types.ts` | Re-exports shared types into the main-process import path |
| `src/main/scheduler/spec.ts` | `compileToCron` + default spec constants; pure functions |
| `src/main/scheduler/spec.test.ts` | Spec compilation + previous-tick math tests |
| `src/main/scheduler/crontab.ts` | Read/write the user crontab; extract and rebuild the managed BEGIN/END block |
| `src/main/scheduler/crontab.test.ts` | Marker round-trip + block-build tests |
| `src/main/scheduler/missed.ts` | Walk-forward expected-tick enumeration via croner's `nextRun`; tolerance match against actual runs |
| `src/main/scheduler/missed.test.ts` | Pure-function tests for missed-run detection |
| `src/main/scheduler/engine.ts` | The engine class: wrapper install, agent scan, crontab sync, manual-run spawn, missed sweep |
| `src/main/scheduler/runs-store.ts` | Reader for `runs/<id>.{json,out}` plus legacy `runs.jsonl`; fs.watch with debounce; lazy output read |
| `src/main/scheduler/schedule-store.ts` | Persists `schedules.json` and `state.json`; serialised through per-file write queues (unchanged) |
| `src/main/scheduler/migrate.ts` | One-shot legacy splitter for the old combined `state.json`; also defines `defaultDataPaths` (now includes `runsDir`) |
| `src/main/agents/scanner.ts` | Discovers executable files under `<userData>/agents/`; reads optional `.meta.json` siblings; seeds the dir on first run |
| `src/main/ipc.ts` | Wires engine + theme to `ipcMain.handle`; defines IPC channel constants |
| `src/main/index.ts` | App boot: builds stores, runs migration, constructs the engine with data/agents/resources dirs, starts theme watcher |

## Data Model

The on-disk shapes are documented under [data-layout](../data-layout/TECHNICAL.md). The summary:

- `schedules.json` — unchanged
- `state.json` — unchanged on disk; `lastFiredAt` is no longer load-bearing for catch-up but is still updated for compatibility
- `runs/<run-id>.json` — meta record per run, written by `wrapper.sh`
- `runs/<run-id>.out` — captured stdout+stderr, written incrementally
- `runs.jsonl` — legacy file, read on boot, never appended after the refactor
- `wrapper.sh` — refreshed on every app start from `resources/wrapper.sh`

## Cron Layout

The managed section sits in the user's crontab between fixed markers:

```
# BEGIN agentic_os (managed - do not edit)
0 9 * * 1,2,3,4,5 '/…/wrapper.sh' '/…/data' 'sched-id' 'job-id' '/…/agents/job-id.sh' # agentic_os:sched-id
# END agentic_os
```

Each cron line carries a trailing `# agentic_os:<scheduleId>` tag. The tag lets the engine recover scheduleId even if BEGIN/END markers are damaged, and makes hand-inspection easy. All four wrapper arguments are single-quoted (with proper `'\''` escaping) so paths with spaces or apostrophes survive cron parsing.

## Noteworthy Behavior

- **`previousRun()` is not used.** croner's `previousRun()` only returns a value if the cron has actually fired in-process — it doesn't compute "what would the prior tick be". `missed.detectMissed` walks forward via `cron.nextRun(cursor)` from `now - windowMs` to `now`, which is reliable.
- **Tolerance is 90s.** Generous to cover cron jitter, machine wake-up lag, and clock skew between wrapper start and the cron scheduled minute. A schedule for `:00` matches a run at `:00:30` or `:00:00.500`. Tighten only if false-positives appear.
- **Missed-run sweep runs even when no UI is open.** It's a setInterval on the engine, fires every 5 minutes, broadcasts only when the missed set changes.
- **fs.watch is debounced 250ms.** When the wrapper writes the running meta and then the final meta, both events would otherwise re-broadcast. The 5-minute sweep also re-indexes the dir as a safety net for any watch event drops.
- **Output is read lazily.** `RunsStore.list()` returns each run with a tail summary (last 4KB) populated at ingest time; full output is only loaded on `scheduler:read-run-output(runId)`. Keeps memory bounded as run history grows.
- **Runs dir is GC'd at engine start.** If `runs/` exceeds 2000 files, the oldest by mtime are deleted down to that cap. No rotation while the app runs.
- **Orphan schedules are *flagged*, not removed.** `engine.listSchedules()` joins `schedule.jobId` against the discovered agents and sets `orphaned: true` for missing matches. Orphans are excluded from crontab sync and rendered in red in the UI.
- **Wrapper is reinstalled on every start.** Cheap, and keeps the on-disk wrapper in lockstep with whatever ships in the asar. Cron lines reference the on-disk path, so this is the only update path needed when the wrapper script changes.
- **Manual runs are detached and unrefed.** `child_process.spawn(wrapper, …, { detached: true, stdio: 'ignore' })` then `cp.unref()` — the run survives even if the user quits the app immediately after clicking.
- **Cron PATH is minimal.** The wrapper exports `PATH=/usr/local/bin:/opt/homebrew/bin:/usr/bin:/bin:$PATH` to give scripts a fighting chance. Anything past that is the script's responsibility.
- **Crontab tampering is detected, not silently overwritten.** `extractManaged` flags duplicate / dangling markers as a conflict; the engine refuses to rewrite until the user invokes `scheduler:reconcile-crontab`, which forcibly purges every BEGIN/END pair and rewrites a clean block.
- **`purgeAllManaged` only removes complete BEGIN..END pairs.** A stray BEGIN with no END (or stray END) deletes only the marker line itself — it never eats trailing user content. This protects against the worst-case "user pasted BEGIN by accident, hit reconcile, lost everything below it" footgun.
- **Crontab read-modify-write is not atomic.** `crontab -l` followed by `crontab -` has a TOCTOU window: another tool (or the user via `crontab -e`) editing the crontab between read and write will be clobbered. POSIX crontab provides no compare-and-swap primitive, so this is unfixable in general. Mitigation in practice: writes happen rarely (only on schedule edits and reconcile), and the trailing `# agentic_os:<scheduleId>` per-line tags let a damaged managed section still be parsed back to the original schedule ids on next sync.
- **`crontab` and `python3` absence are surfaced, not hidden.** `engine.start()` checks both with `--version`; `crontabStatus` returns `crontabOk` / `pythonOk` / `wrapperOk` flags; the dashboard's `SystemBanner` renders an actionable error if anything's missing.

## Dependencies

- `croner` — still used for `compileToCron` source and `nextRun()` in `missed.detectMissed` and `engine.nextRunFor`
- `child_process` (Node) — `crontab -l`, `crontab -`, `python3 --version`, manual-run wrapper spawn
- `fs.watch` (Node) — runs directory observer
- `python3` (system) — required at runtime to JSON-encode wrapper meta files
- `crontab` (system) — required at runtime to read/write the user crontab (Linux: `cronie` or similar; macOS ships it)
- `electron` (main only) — `app.getPath('userData')`, `ipcMain`, `shell.openPath`
