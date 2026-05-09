# Scheduler Engine

A thin layer on top of system cron. Schedules are stored as `hourly` or `daily` specs, compiled to cron expressions, and installed into a managed section of the user's crontab. A small bash wrapper invoked by cron records each run to `<userData>/data/runs/`. The Electron app discovers agents (executable files under `<userData>/agents/`), reconciles the crontab on schedule edits, watches the runs directory, and sweeps for missed runs every five minutes. There is no in-process ticker — runs fire whether or not the app is open.

## Documents

| Document | Purpose |
|----------|---------|
| [DESIGN.md](DESIGN.md) | What the engine guarantees, the wrapper contract, missed-run handling, key trade-offs |
| [TECHNICAL.md](TECHNICAL.md) | Source files, crontab layout, missed-run algorithm, runtime dependencies |
| [FLOW.mermaid](FLOW.mermaid) | Run lifecycle (cron and manual triggers, status transitions) |
