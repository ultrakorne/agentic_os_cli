# Data Layout — Technical

## Architecture

`src/main/index.ts` resolves three roots on app ready:

- `dataDir = app.getPath('userData') + '/data'` — schedules, run logs, wrapper, state
- `agentsDir = app.getPath('userData') + '/agents'` — user-owned executable scripts
- `resourcesDir` — bundled assets (`wrapper.sh`, seed agents); resolved differently in dev vs packaged builds

`defaultDataPaths(dataDir)` returns the four canonical paths under `data/` (schedules, state, legacy `runs.jsonl`, and the new `runs/` directory). Before constructing stores, `migrateLegacyStateIfNeeded` runs (legacy combined `state.json` splitter — unchanged from earlier versions). The two stores then `load()`, the engine installs the wrapper into `dataDir`, seeds the agents dir if empty, scans agents, reconciles the crontab, and starts watching the runs directory.

## Source Files

| File | Role |
|------|------|
| `src/main/scheduler/migrate.ts` | `defaultDataPaths` (now includes `runsDir`) + `migrateLegacyStateIfNeeded` |
| `src/main/scheduler/schedule-store.ts` | Owns `schedules.json` + `state.json`; atomic-rename writes, per-file queues |
| `src/main/scheduler/runs-store.ts` | Reader: indexes `<dataDir>/runs/<id>.json` files; reads legacy `runs.jsonl`; fs.watch + debounce; lazy output read |
| `src/main/agents/scanner.ts` | Lists executable files under `<userData>/agents/`; reads optional `.meta.json` siblings; seeds dir on first run |
| `src/main/index.ts` | Resolves the three roots, runs migration, wires the stores into the engine |

## Data Model

### `schedules.json`

```json
{
  "schedules": [
    { "id": "ping", "jobId": "ping", "spec": { "kind": "hourly", "everyHours": 1, "minute": 0 } },
    { "id": "morning-digest", "jobId": "morning-digest", "spec": { "kind": "daily", "days": ["mon","tue","wed","thu","fri"], "hour": 9, "minute": 0 } }
  ]
}
```

`Schedule` and `ScheduleSpec` are defined in `src/shared/scheduler.ts`. The `orphaned: boolean` field is computed at read time by joining `jobId` against the discovered agents — it is **never** persisted.

### `state.json`

```json
{
  "state": {
    "ping": { "lastFiredAt": "2026-05-08T15:00:00.000Z" }
  }
}
```

`lastFiredAt` is preserved for backward compatibility but no longer drives any code path. The engine does not write to it under the cron-based architecture; legacy values stay on disk.

### `runs/<run-id>.json`

One JSON object per run, written atomically (temp + rename) by `wrapper.sh`:

```json
{
  "id": "1778333992-12627-163598403",
  "jobId": "ping",
  "scheduleId": "ping",
  "trigger": "schedule",
  "startedAt": "2026-05-09T13:39:52.207Z",
  "endedAt": "2026-05-09T13:39:52.236Z",
  "status": "success",
  "exitCode": 0,
  "output": "",
  "error": null,
  "outputPath": "1778333992-12627-163598403.out"
}
```

The wrapper writes a `running` meta record first, then rewrites with `endedAt` / `status` / `exitCode` after the script completes. `outputPath` is relative to the runs directory.

### `runs/<run-id>.out`

Combined stdout+stderr from the agent. Written incrementally by the wrapper via shell redirection. Read lazily by the main process — `RunsStore` populates a tail summary (last ~4KB) into `JobRun.output` at ingest time, and serves the full file on demand via the `scheduler:read-run-output` IPC method.

### `runs.jsonl` (legacy)

Old append-only file from before the refactor. Read once at boot into the in-memory cache as historical data; never appended again. New runs land only in `runs/`.

### `wrapper.sh`

Copied from `resources/wrapper.sh` into `<dataDir>/wrapper.sh` on every engine start, then `chmod 755`. The on-disk path is what the managed crontab section references — keeping it stable across upgrades.

### `<userData>/agents/`

User-owned scripts. The filename without extension is the agent id (`ping.sh` → `ping`). Optional sibling `<id>.meta.json` supplies `{ title, description, section }`. On first run the engine copies seed agents (`ping.sh`, `disk-free.sh`, `README.md`) from `resources/agents/`.

## Noteworthy Behavior

- **Per-run files, not jsonl appends.** Each run creates two files (`<id>.json` + `<id>.out`). Atomic via temp + rename for the meta; raw redirection for the output. No flock contention, no escaping headaches in bash.
- **fs.watch on `runs/`** is debounced 250ms; the 5-minute missed-run sweep doubles as a safety net for any watch events the OS drops.
- **GC at engine start, not continuous.** If `runs/` exceeds 2000 files, the oldest by mtime are deleted down to that cap. No automatic rotation while the app runs.
- **In-memory cache is 500 newest runs.** Plus a tail summary per run (~4KB). Renderer-visible memory stays bounded even as the on-disk run count grows toward the 2000-file cap.
- **Atomic-rename for the JSON files**, raw `appendFile` for nothing (the legacy JSONL is read-only now). `writeJson` writes `path.tmp` then `rename`s.
- **Per-file write queues** still serialise mutations in `ScheduleStore`. `RunsStore` doesn't need a write queue — it's a reader.
- **Legacy migration is detected by content shape.** `isLegacyShape` checks for `schedules` or `runs` at the root of `state.json`. Unchanged from before; old combined `state.json` is renamed to `state.json.legacy` rather than deleted.
- **Missing files are not errors.** All stores treat `ENOENT` as "first run, start empty"; the migrator treats `ENOENT` on `state.json` as "nothing to migrate"; the agents scanner treats `ENOENT` on the agents dir as "create then return empty".

## Dependencies

- `node:fs/promises` and `node:fs.watch` — store persistence and runs-dir observation
- `electron.app.getPath('userData')` — platform-correct data dir (`~/.config/agentic-os/` on Linux, `~/Library/Application Support/agentic-os/` on macOS)
- `python3` (system) — required by `wrapper.sh` to JSON-encode meta records
- `crontab` (system) — required to read/write the user crontab
