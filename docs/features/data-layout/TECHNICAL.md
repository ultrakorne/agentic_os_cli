# Data Layout — Technical

## Architecture

`src/main/index.ts` resolves two roots on app ready:

- `dataDir = app.getPath('userData') + '/data'` — engine state and user scripts (the latter under `dataDir/agents/`)
- `resourcesDir` — bundled assets (`wrapper.sh`, seed agents); resolved differently in dev vs packaged builds

The engine then indexes the runs directory, installs the wrapper, seeds the agents dir if empty, scans agents (which folds in each script's `.meta.json` sidecar in the same pass), reconciles the crontab, and starts watching the runs directory. There is no global config file to load.

## Source Files

| File | Role |
|------|------|
| `src/main/scheduler/agent-meta-store.ts` | Reads/writes per-agent `<id>.meta.json` sidecars; per-path write queue; atomic temp+rename; an empty meta deletes the sidecar |
| `src/main/scheduler/runs-store.ts` | Reader: indexes `<dataDir>/runs/<id>.json` files; fs.watch + debounce; lazy output read; pair-aware GC |
| `src/main/agents/scanner.ts` | Walks `<dataDir>/agents/` (top-level + first-level subdirs); attributes section from parent folder; deduplicates ids; reads each `<id>.meta.json` sidecar inline |
| `src/main/index.ts` | Resolves the roots and wires the stores into the engine |

## Data Model

### `<id>.meta.json` (sidecar)

```json
{
  "schedule": { "kind": "hourly", "everyHours": 1, "minute": 0 },
  "scheduledAt": "2026-05-09T13:39:52Z",
  "title": "Morning digest",
  "description": "Summarize overnight notifications"
}
```

`AgentMeta` is defined in `src/shared/scheduler.ts`. Every field is optional; a missing or empty sidecar means defaults all the way down (no schedule, humanized id as title, blank description). The id and section are derived from the script's path and are never stored in the sidecar.

The dashboard's unified `Agent` view (returned by `agents:list`) is built directly from each scanned `AgentEntry`:

```ts
type Agent = {
  id: string
  title: string         // meta.title ?? humanize(id)
  description: string
  section: string       // from disk
  scriptPath: string
  schedule?: ScheduleSpec
  scheduledAt?: string
  scheduled: boolean
}
```

A sidecar with no matching executable script is silently ignored on scan — it does not appear in `agents:list`. (If the user later adds the script, the sidecar attaches automatically.)

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

### `<dataDir>/workspaces/`

Optional. Free-form per-agent working directories — prompt files, state an
agent reads or writes, caches, anything an agent needs that isn't part of
the script itself. Not scanned, not seeded, never required. Agents
reference workspaces via the `$AGENTIC_OS_DATA_DIR/workspaces/<name>` env
var exported by the wrapper, so the same script works on Linux and macOS
without hard-coded paths.

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
- **Atomic-rename for the JSON files.** `writeJson` writes `path.tmp` then `rename`s. Sidecar writes that produce an empty meta `unlink` the file instead, so the directory listing stays minimal.
- **Per-path write queue** in `AgentMetaStore` serialises mutations to the same sidecar; different sidecars write concurrently.
- **Missing files are not errors.** All readers treat `ENOENT` as "no meta, use defaults"; the agents scanner creates the dir on first call.
- **Scanner skips `*.meta.json`.** Sidecars sit next to scripts in the same directory; the executable walk filters them out so a meta file is never mistaken for an agent.

## Dependencies

- `node:fs/promises` and `node:fs.watch` — store persistence and runs-dir observation
- `electron.app.getPath('userData')` — platform-correct data dir (`~/.config/agentic-os/` on Linux, `~/Library/Application Support/agentic-os/` on macOS)
- `python3` (system) — required by `wrapper.sh` to JSON-encode meta records
- `crontab` (system) — required to read/write the user crontab
