# Agentic OS Documentation

Personal desktop dashboard that turns scattered agents, scripts, and skills into a single curated surface. The single owner clicks to launch agents or schedules them; agents are user-owned shell scripts run by **system cron**, so they fire whether or not the app is open. Aesthetic is retro-runtime (synthwave-CRT crossed with terminal-vim) driven entirely by the active Omarchy theme. See [PRODUCT.md](../PRODUCT.md) for the product brief.

## Architecture

Agentic OS is a **view over filesystem state**. The renderer reads JSON files under `<userData>/data/` and shows them. Heavy work lives in three layers below the view:

1. **System cron is the ticker.** Scheduled runs fire from a managed section of the user's crontab, not from a long-lived process inside the app. Schedules survive reboots, app crashes, and weeks of the dashboard never being opened.
2. **`wrapper.sh` is the executor.** Cron invokes it; it runs the agent script and writes one meta JSON + one `.out` file per run into `<userData>/data/runs/`. Manual "run now" from the UI spawns the same wrapper detached, so cron triggers and click triggers travel the exact same code path.
3. **An engine tick keeps state coherent.** Re-scans the agents directory (folding in each script's `<id>.meta.json` sidecar), reconciles the managed crontab against the discovered schedules, sweeps for missed runs. The tick currently runs in-process while the GUI is open (every five minutes); it is designed to also be driveable from a single cron line — `*/5 * * * * agentic-os tick` — so the engine stays correct when the app has not been opened in days.

Consequences worth defending:

- **Adding an agent is dropping an executable file** into `<userData>/data/agents/`. No reload, no rebuild, no in-tree registry. The next tick (or next GUI open) picks it up. Scheduling is what makes cron care; until then a script is purely visible, not active.
- **The view is closeable.** The UI is a way to see and edit; nothing in the runtime contract depends on it being alive.
- **A future TUI/CLI is just another view over the same files.** Every UI mutates per-agent `<id>.meta.json` sidecars and reads `runs/*.json`; there is no business logic to port.
- **New features default to filesystem first.** Store as JSON, mutate via small focused scripts, render in the view. Reach for renderer-side logic only when the work is genuinely interactive (drag, animation, micro-state).

When in doubt: if a piece of work can run from cron without the app open, it should.

## Tech Stack

- **Shell**: Electron 39 (main + preload + renderer), packaged with electron-vite + electron-builder
- **Renderer**: React 19 + TypeScript, Zustand for state, Tailwind 4 (CSS-first `@theme`)
- **Scheduling**: System `cron` is the ticker; the app manages a section of the user's crontab and reads run logs. `croner` is used only for cron-string compilation and `nextRun` calculations.
- **Run capture**: A bash wrapper script invoked by every cron line (and by every manual "run now") writes one meta JSON + one stdout file per run.
- **Persistence**: Plain JSON files under Electron's `userData/data/`; user-owned executable scripts under `userData/data/agents/`.
- **Theming**: Reads Omarchy's `~/.config/omarchy/current/theme/colors.toml` at runtime, with a watcher
- **Tests**: Vitest (unit tests live next to the code: `*.test.ts`)
- **Runtime requirements**: `crontab` (Linux: `cronie` or similar; macOS ships it) and `python3` on PATH.

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
