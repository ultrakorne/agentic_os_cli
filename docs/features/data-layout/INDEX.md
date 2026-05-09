# Data Layout

The on-disk shape of the app's state. Three files under Electron's `userData/data/` directory: `schedules.json` (the schedules), `state.json` (per-schedule `lastFiredAt` for catch-up), `runs.jsonl` (append-only run history). A one-shot migrator splits an older combined `state.json` into this three-file layout. Plus `ping.log` from the `ping` dev agent. The renderer never reads or writes these files directly — everything goes through the engine over IPC.

## Documents

| Document | Purpose |
|----------|---------|
| [DESIGN.md](DESIGN.md) | Why three files, why JSON/JSONL, ownership rules |
| [TECHNICAL.md](TECHNICAL.md) | Paths, file shapes, write semantics, legacy migration |
