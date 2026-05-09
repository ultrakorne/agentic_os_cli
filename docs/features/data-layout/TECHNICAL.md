# Data Layout — Technical

## Architecture

`src/main/index.ts` resolves two roots on app ready:

- `dataDir = app.getPath('userData') + '/data'` — engine state and user scripts (the latter under `dataDir/agents/`)
- `resourcesDir` — bundled assets (`wrapper.sh`, seed agents); resolved differently in dev vs packaged builds

The engine then loads `agents.json`, indexes the runs directory, installs the wrapper, seeds the agents dir if empty, scans agents, reconciles the crontab, and starts watching the runs directory.

## Source Files

| File | Role |
|------|------|
| `src/main/scheduler/agent-config-store.ts` | Owns `agents.json`; atomic-rename writes, single write queue; `setSchedule(id, spec | null)` is the main mutation |
| `src/main/scheduler/runs-store.ts` | Reader: indexes `<dataDir>/runs/<id>.json` files; fs.watch + debounce; lazy output read; pair-aware GC |
| `src/main/agents/scanner.ts` | Walks `<dataDir>/agents/` (top-level + first-level subdirs); attributes section from parent folder; deduplicates ids |
| `src/main/index.ts` | Resolves the roots and wires the stores into the engine |

## Data Model

### `agents.json`

```json
{
  "agents": [
    {
      "id": "ping",
      "schedule": { "kind": "hourly", "everyHours": 1, "minute": 0 }
    },
    {
      "id": "morning-digest",
      "title": "Morning digest",
      "description": "Summarize overnight notifications",
      "schedule": { "kind": "daily", "days": ["mon","tue","wed","thu","fri"], "hour": 9, "minute": 0 }
    },
    {
      "id": "release-notes"
    }
  ]
}
```

`AgentConfig` is defined in `src/shared/scheduler.ts`. Every field except `id` is optional. The dashboard typically writes only `id + schedule`; `title` and `description` are populated by hand-edit. Section is **never** stored here — it's read from the script's parent directory at scan time.

The dashboard's unified `Agent` view (returned by `agents:list`) is computed at IPC time by joining this file against the scanned scripts:

```ts
type Agent = {
  id: string
  title: string         // resolved (config.title || humanize(id))
  description: string
  section: string       // from disk
  scriptPath?: string   // undefined = orphan config entry
  schedule?: ScheduleSpec
  scheduled: boolean
  orphaned: boolean
}
```

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

Combined stdout+stderr from the agent. Written incrementally by shell redirection. Read lazily by the main process — `RunsStore` populates a tail summary (last ~4KB) into `JobRun.output` at ingest time, full file served on demand via `scheduler:read-run-output`.

### `wrapper.sh`

Copied from `resources/wrapper.sh` into `<dataDir>/wrapper.sh` on every engine start, then `chmod 755`. The on-disk path is what the managed crontab section references.

### `<dataDir>/agents/`

User-owned scripts. Scanned with this layout:

```
data/agents/
  ping.sh                       → id="ping",      section="Agents"
  Daily/morning-digest.sh       → id="morning-digest", section="Daily"
  Engineering/pr-watch.sh       → id="pr-watch",  section="Engineering"
  .hidden/ignored.sh            → skipped (dot-prefixed)
  Daily/sub/too-deep.sh         → skipped (deeper than first level)
```

IDs must be unique across the whole tree; duplicates are dropped (top-level wins, then alphabetical) with a `[scanner]` console warning.

## Noteworthy Behavior

- **Per-run files, not jsonl appends.** Each run creates two files (`<id>.json` + `<id>.out`). Atomic via temp + rename for the meta; raw redirection for the output.
- **fs.watch on `runs/`** is debounced 250ms; the 5-minute missed-run sweep doubles as a safety net for any watch events the OS drops.
- **GC at engine start, not continuous.** If `runs/` exceeds 2000 runs, the oldest by mtime are deleted down to that cap. Files are grouped by stem so `<id>.json` and `<id>.out` always age out together — never an orphan on either side.
- **In-memory cache is 500 newest runs.** Plus a tail summary per run (~4KB).
- **Atomic-rename for the JSON files.** `writeJson` writes `path.tmp` then `rename`s.
- **Single write queue** in `AgentConfigStore` serialises mutations.
- **Missing files are not errors.** All readers treat `ENOENT` as "first run, start empty"; the agents scanner creates the dir on first call.

## Dependencies

- `node:fs/promises` and `node:fs.watch` — store persistence and runs-dir observation
- `electron.app.getPath('userData')` — platform-correct data dir (`~/.config/agentic-os/` on Linux, `~/Library/Application Support/agentic-os/` on macOS)
- `python3` (system) — required by `wrapper.sh` to JSON-encode meta records
- `crontab` (system) — required to read/write the user crontab
