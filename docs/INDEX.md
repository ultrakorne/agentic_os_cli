# Agentic OS Documentation

Personal desktop dashboard that turns scattered agents, scripts, and skills into a single curated surface. The single owner clicks to launch agents or schedules them; agents are user-owned shell scripts run by **system cron**, so they fire whether or not the app is open. Aesthetic is retro-runtime (synthwave-CRT crossed with terminal-vim) driven entirely by the active Omarchy theme. See [PRODUCT.md](../PRODUCT.md) for the product brief.

## Architecture

Agentic OS is a **view over filesystem state**. The runtime lives in the [`aos` CLI](../agentic_os_cli/) (Go); the Electron app reads files from `<aos_home>/` and renders them. Three layers sit below the view:

1. **System cron is the ticker.** Scheduled runs fire from a managed section of the user's crontab, not from a long-lived process inside the app. Schedules survive reboots, app crashes, and weeks of the dashboard never being opened.
2. **`wrapper.sh` is the executor.** Cron invokes it; it runs the agent script and writes one meta JSON + one `.out` file per run into `<aos_home>/runs/`. Manual "run now" from the UI spawns the same wrapper detached, so cron triggers and click triggers travel the exact same code path.
3. **`aos refresh` keeps cron coherent.** Re-scans the agents directory (folding in each script's `<id>.meta.json` sidecar), reconciles the managed crontab against the discovered schedules. Fires from cron itself every 10 minutes via the `__tick__` entry, plus on demand whenever the dashboard mutates a meta file or the user clicks "refresh".

Consequences worth defending:

- **The CLI owns everything that touches the system.** `wrapper.sh`, the cron block, the tick log — all managed by `aos`. If `aos` is not on PATH, the dashboard renders an install banner and otherwise does nothing.
- **Adding an agent is dropping an executable file** into `<aos_home>/agents/`. No reload, no rebuild, no in-tree registry. The next tick (or next refresh) picks it up.
- **The view is closeable.** Cron keeps firing whether or not the dashboard is open.
- **A future TUI is just another view over the same files.** Every UI mutates per-agent `<id>.meta.json` sidecars and reads `runs/*.json`; there is no business logic to port.
- **New features default to filesystem first.** Store as JSON, mutate via small focused scripts, render in the view. Reach for renderer-side logic only when the work is genuinely interactive (drag, animation, micro-state).

When in doubt: if a piece of work can run from cron without the app open, it should — and that means it lives in the CLI.

## Tech Stack

- **Runtime CLI**: Go (`aos`) — wraps cron, owns `wrapper.sh`, manages the managed crontab block, prints the aos_home path via `aos home`.
- **Shell**: Electron 39 (main + preload + renderer), packaged with electron-vite + electron-builder
- **Renderer**: React 19 + TypeScript, Zustand for state, Tailwind 4 (CSS-first `@theme`)
- **Scheduling**: System `cron` is the ticker; `aos` manages a section of the user's crontab. `croner` is used in the dashboard only for `nextRun` calculations.
- **Run capture**: A bash wrapper script (`wrapper.sh`, installed by `aos init`) invoked by every cron line and by every manual "run now" writes one meta JSON + one stdout file per run.
- **Persistence**: Plain JSON files under `<aos_home>/`; user-owned executable scripts under `<aos_home>/agents/`. `aos_home` defaults to wherever you passed to `aos init` (e.g. `~/.aos`).
- **Theming**: Reads Omarchy's `~/.config/omarchy/current/theme/colors.toml` at runtime, with a watcher
- **Tests**: Vitest for the dashboard (`*.test.ts`), `go test` for the CLI (`*_test.go`)
- **Runtime requirements**: `aos` CLI on PATH, `crontab` (Linux: `cronie` or similar; macOS ships it), and `python3` on PATH.

## Features

| Feature | Description |
|---------|-------------|
| [scheduler-engine](features/scheduler-engine/INDEX.md) | Cron-backed scheduler: managed crontab section, wrapper script, missed-run detection, manual re-run |
| [theme-system](features/theme-system/INDEX.md) | Live Omarchy theme load + watch, exposed to the renderer as CSS variables |
| [data-layout](features/data-layout/INDEX.md) | On-disk file layout for agent config, run logs, and user scripts |
| [dashboard-ui](features/dashboard-ui/INDEX.md) | React dashboard: sectioned agent cards, run buttons, missed-runs banner, inline schedule editor |

## Quick Links

- [Product brief](../PRODUCT.md)
- [Getting started](../README.md)
