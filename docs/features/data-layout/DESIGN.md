# Data Layout — Design

## Overview

Single-user app, plain files, no database. Everything lives under `<userData>/data/`. State splits along three axes that have different write patterns and durability needs: agent config (rare writes, hand-edit-friendly), run history (one file per run, written by an external process), and user-owned scripts (managed by the owner directly). Each gets its own location.

## Components

### `data/agents.json`

The unified agent config: `{ agents: AgentConfig[] }`. Each entry holds the agent id plus any combination of `schedule`, `title`, `description`. Hand-editable. Rewritten in full whenever the user edits a schedule. Section is **never** in this file — it comes from the script's parent folder.

### `data/agents/<id>.<ext>` or `data/agents/<Section>/<id>.<ext>`

User-owned executable scripts. The filename minus extension is the agent id. The parent directory determines the section: top-level scripts get section "Agents"; a first-level subdirectory name becomes the section. Adding an agent means dropping a script; deleting one means removing the file; reorganizing means `mv`.

### `data/runs/<run-id>.json` + `<run-id>.out`

Run history, two files per run. The meta JSON (`<id>.json`) is written atomically by the wrapper — first as a `running` placeholder, then rewritten with the final status + exit code. The output (`<id>.out`) is the raw combined stdout+stderr captured by shell redirection. The owner can `tail` either file directly.

### `data/wrapper.sh`

The bash wrapper invoked by every cron line and by every manual "run now". Refreshed from the bundled `resources/wrapper.sh` on every app start. The on-disk path is what crontab lines reference — keeping a stable file path across app upgrades.

## Design Decisions

- **Everything under `data/`.** Both engine state and user scripts live there. One directory to back up, one path to inspect.
- **Single agents.json**, not one file per agent. One file you can hand-edit, version-control, or copy to another machine.
- **Section from filesystem, not config.** The agent's place in the dashboard is determined by where the script lives on disk. Reorganizing is just `mv`. There's no risk of the config saying "Daily" while the disk says nothing — only one place to look.
- **Per-run files, not a single jsonl.** Atomic writes via temp + rename. Multiline output goes to a sibling `.out` file so JSON escaping in bash stays bounded to fixed fields. Two files per run is a small price for not having to reason about flock or torn appends.
- **Atomic writes for the JSON files.** Write to `path.tmp`, rename over `path`. Guarantees a partially-written file never replaces a good one.
- **External writer (cron + wrapper), internal reader (engine + UI).** The Electron main process is a reader of the runs directory rather than the source of truth. `fs.watch` keeps the UI live; periodic re-indexing is a safety net.
- **Bounded in-memory cache.** Newest 500 runs hot, with a tail summary per run (~4KB). Older entries stay on disk; full output is loaded on demand. A hard cap of 2000 runs in `runs/` is enforced at engine start (`<id>.json` + `<id>.out` paired so neither orphans).
- **No backups, no rotation, no sync.** Out of scope. The owner can copy `data/` if they care.
