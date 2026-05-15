# Data Layout

The on-disk shape of the runtime state lives under `<aos_home>/` (the path
the user passed to `aos init`, e.g. `~/.aos`). The dashboard discovers this
location by calling `aos home`.

- `agents/<id>.<ext>` — user-owned executable shell scripts; section determined by parent folder
- `agents/<id>.meta.json` — optional sidecar with that agent's schedule, title, description (written when the dashboard edits it)
- `workspaces/` — optional per-agent working dirs (prompts, state, caches); referenced by scripts via `$AGENTIC_OS_DATA_DIR/workspaces/<name>`
- `runs/` — one pair of `<run-id>.{json,out}` per run
- `wrapper.sh` — installed by `aos init` from the CLI's embedded copy
- `tick.log` — append-only log of `aos tick` invocations (trimmed by `aos refresh`)

The Electron renderer never reads these paths directly — main-process IPC fronts them. The Go CLI is the only code that mutates `wrapper.sh` or the user's crontab.

Config:

- `~/.config/aos/config.toml` — single-key TOML pointing at the chosen aos_home

## Documents

| Document | Purpose |
|----------|---------|
| [DESIGN.md](DESIGN.md) | What each file is for, write strategies |
| [TECHNICAL.md](TECHNICAL.md) | File-by-file shapes, on-disk semantics |
