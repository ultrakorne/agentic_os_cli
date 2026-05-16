# Scheduler Engine

A thin layer on top of system cron. Schedules are stored as `hourly` or `daily` specs (in per-agent `<id>.meta.json` sidecars), compiled to cron expressions, and installed into a managed section of the user's crontab. A small bash wrapper invoked by cron records each run to `<aos_home>/runs/`.

> **Note (2026-05):** Cron and wrapper management have moved out of the Electron app and into the [`aos` CLI](../../../agentic_os_cli/), which is a sibling project that owns its own documentation. The DESIGN.md / TECHNICAL.md / FLOW.mermaid below describe the older in-process design and are partly stale on the runtime side; the CLI is the current source of truth for anything that touches the user's system (cron, wrapper, sidecar writes, tick).

## Documents

| Document | Purpose |
|----------|---------|
| [DESIGN.md](DESIGN.md) | What the engine guarantees, the wrapper contract, missed-run handling, key trade-offs |
| [TECHNICAL.md](TECHNICAL.md) | Source files, crontab layout, missed-run algorithm, runtime dependencies |
| [FLOW.mermaid](FLOW.mermaid) | Run lifecycle (cron and manual triggers, status transitions) |
