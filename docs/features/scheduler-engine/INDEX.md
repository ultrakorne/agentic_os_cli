# Scheduler Engine

A thin layer on top of system cron. Schedules are stored as `hourly` or `daily` specs (in per-agent `<id>.meta.json` sidecars), compiled to cron expressions, and installed into a managed section of the user's crontab. A small bash wrapper invoked by cron records each run to `<aos_home>/runs/`.

> **Note (2026-05):** Cron/wrapper management moved out of the Electron app and into the [`aos` CLI](../../../agentic_os_cli/). The CLI handles `aos init`, `aos refresh` (reconcile cron), and `aos tick` (fired every 10 min by cron itself). The Electron app now scans agents, computes missed runs, writes meta sidecars, and shells out to `aos refresh` after any change. DESIGN.md / TECHNICAL.md / FLOW.mermaid below describe the older in-process design and are partly stale; see [`agentic_os_cli/AGENTS.md`](../../../agentic_os_cli/AGENTS.md) for the current truth.

## Documents

| Document | Purpose |
|----------|---------|
| [DESIGN.md](DESIGN.md) | What the engine guarantees, the wrapper contract, missed-run handling, key trade-offs |
| [TECHNICAL.md](TECHNICAL.md) | Source files, crontab layout, missed-run algorithm, runtime dependencies |
| [FLOW.mermaid](FLOW.mermaid) | Run lifecycle (cron and manual triggers, status transitions) |
