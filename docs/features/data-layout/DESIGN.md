# Data Layout — Design

## Overview

Single-user app, plain files, no database. Everything lives under `<userData>/data/`. State splits along three axes that have different write patterns and durability needs: per-agent meta (rare writes, hand-edit-friendly, lives next to the script), run history (one file per run, written by an external process), and user-owned scripts (managed by the owner directly). Each gets its own location.

## Components

### `data/agents/<id>.<ext>` or `data/agents/<Section>/<id>.<ext>`

User-owned executable scripts. The filename minus extension is the agent id. The parent directory determines the section: top-level scripts get section "Agents"; a first-level subdirectory name becomes the section. Adding an agent means dropping a script; deleting one means removing the file; reorganizing means `mv`.

### `data/agents/<id>.meta.json` (sidecar)

Optional. Holds whatever the dashboard learns *about* an agent that doesn't belong on disk: `schedule`, `scheduledAt`, `title`, `description`. The id and section are derived from the script's path, never stored here. Written via temp+rename when the user edits a schedule or description. An empty meta is removed rather than left as `{}`. Hand-editable.

### `data/runs/<run-id>.json` + `<run-id>.out`

Run history, two files per run. The meta JSON (`<id>.json`) is written atomically by the wrapper — first as a `running` placeholder, then rewritten with the final status + exit code. The output (`<id>.out`) is the raw combined stdout+stderr captured by shell redirection. The owner can `tail` either file directly.

### `data/wrapper.sh`

The bash wrapper invoked by every cron line and by every manual "run now". Refreshed from the bundled `resources/wrapper.sh` on every app start. The on-disk path is what crontab lines reference — keeping a stable file path across app upgrades.

## Design Decisions

- **Everything under `data/`.** Both engine state and user scripts live there. One directory to back up, one path to inspect.
- **Per-script sidecars, not a central registry.** Meta lives next to the script as `<id>.meta.json`. Copy a script and its sidecar together to share an agent; `rm <id>.*` cleans up everything in one step. There is no "orphan config" failure mode — if there's no script, there's no agent.
- **Section from filesystem, not config.** The agent's place in the dashboard is determined by where the script lives on disk. Reorganizing is just `mv`. There's no risk of the config saying "Daily" while the disk says nothing — only one place to look.
- **Per-run files, not a single jsonl.** Atomic writes via temp + rename. Multiline output goes to a sibling `.out` file so JSON escaping in bash stays bounded to fixed fields. Two files per run is a small price for not having to reason about flock or torn appends.
- **Atomic writes for the JSON files.** Write to `path.tmp`, rename over `path`. Guarantees a partially-written file never replaces a good one.
- **External writer (cron + wrapper), internal reader (engine + UI).** The Electron main process is a reader of the runs directory rather than the source of truth. `fs.watch` keeps the UI live; periodic re-indexing is a safety net.
- **Bounded in-memory cache.** Newest 500 runs hot, with a tail summary per run (~4KB). Older entries stay on disk; full output is loaded on demand. A hard cap of 2000 runs in `runs/` is enforced at engine start (`<id>.json` + `<id>.out` paired so neither orphans).
- **No backups, no rotation, no sync.** Out of scope. The owner can copy `data/` if they care.
