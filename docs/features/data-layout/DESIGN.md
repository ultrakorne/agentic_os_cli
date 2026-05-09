# Data Layout — Design

## Overview

Single-user app, plain files, no database. State splits along two axes that have different write patterns and different durability needs: schedules (rare writes, hand-edit-friendly), and run history (one file per run, written by an external process). User-owned executable scripts (the "agents") live in their own directory. Each axis gets its own location.

## Components

### `data/schedules.json`

The schedule list, one JSON object: `{ schedules: Schedule[] }`. Hand-editable. Rewritten in full on any add / remove / change. The durable record of "what does the owner want to run".

### `data/state.json`

Per-schedule firing state, kept for backward compatibility with earlier app versions. Not load-bearing under the cron-based architecture; cron + run files are the source of truth now.

### `data/runs/<run-id>.json` + `<run-id>.out`

Run history, two files per run. The meta JSON (`<id>.json`) is written atomically by the wrapper — first as a `running` placeholder, then rewritten with the final status + exit code. The output (`<id>.out`) is the raw combined stdout+stderr, captured by shell redirection. The owner can `tail` either file directly.

### `data/runs.jsonl` (legacy)

Old append-only history file from before the cron refactor. Read once on boot into the historical cache, never appended after. Kept on disk so old runs remain visible.

### `data/wrapper.sh`

The bash wrapper invoked by every cron line and by every manual "run now". Refreshed from the bundled `resources/wrapper.sh` on every app start. The on-disk path is what crontab lines reference — keeping a stable file path across app upgrades.

### `agents/<id>.sh` (and other extensions) plus optional `<id>.meta.json`

User-owned executable scripts. The filename minus extension is the agent id. Adding an agent means dropping a script; deleting one means removing the file. Optional sibling `.meta.json` supplies display metadata.

## Design Decisions

- **Two homes for state, not one.** `data/` is engine-owned (schedules, runs, wrapper). `agents/` is user-owned (scripts). Keeping them separate makes it obvious what the owner can hand-edit (everything in `agents/`) and what they shouldn't (everything in `data/`).
- **Per-run files, not a single jsonl.** Atomic writes via temp + rename. Multiline output goes to a sibling `.out` file so JSON escaping in bash stays bounded to fixed fields. Two files per run is a small price to pay for not having to reason about flock or torn appends.
- **Atomic writes for the JSON files.** Write to `path.tmp`, rename over `path`. Guarantees a partially-written file never replaces a good one.
- **External writer (cron + wrapper), internal reader (engine + UI).** The Electron main process becomes a reader of the runs directory rather than the source of truth. `fs.watch` keeps the UI live; periodic re-indexing is a safety net.
- **Bounded in-memory cache.** Newest 500 runs hot, with a tail summary per run (~4KB). Older entries stay on disk; full output is loaded on demand. A hard cap of 2000 files in `runs/` is enforced at engine start (oldest by mtime deleted).
- **Migration on boot, not on demand.** Legacy combined `state.json` is split into the new layout exactly once; legacy `runs.jsonl` is read but never appended. Future schema changes follow the same pattern.
- **No backups, no rotation, no sync.** Out of scope. The owner can copy `data/` and `agents/` if they care.
