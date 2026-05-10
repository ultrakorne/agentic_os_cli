# Data Layout

The on-disk shape of the app's state, all under `<userData>/data/`:

- `agents/<id>.<ext>` — user-owned executable shell scripts; section determined by parent folder
- `agents/<id>.meta.json` — optional sidecar with that agent's schedule, title, description (written when the dashboard edits it)
- `workspaces/` — optional per-agent working dirs (prompts, state, caches); referenced by scripts via `$AGENTIC_OS_DATA_DIR/workspaces/<name>`
- `runs/` — one pair of `<run-id>.{json,out}` per run
- `wrapper.sh` — refreshed from the bundled resource on every app start

The renderer never reads or writes these paths directly — everything goes through the engine over IPC.

## Documents

| Document | Purpose |
|----------|---------|
| [DESIGN.md](DESIGN.md) | What each file is for, write strategies |
| [TECHNICAL.md](TECHNICAL.md) | File-by-file shapes, on-disk semantics |
