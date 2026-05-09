# Agentic OS Documentation

Personal desktop dashboard that turns scattered agents, scripts, and skills into a single curated surface. The single owner clicks to launch agents or schedules them; agents are user-owned shell scripts run by **system cron**, so they fire whether or not the app is open. Aesthetic is retro-runtime (synthwave-CRT crossed with terminal-vim) driven entirely by the active Omarchy theme. See [PRODUCT.md](../PRODUCT.md) for the product brief.

## Tech Stack

- **Shell**: Electron 39 (main + preload + renderer), packaged with electron-vite + electron-builder
- **Renderer**: React 19 + TypeScript, Zustand for state, Tailwind 4 (CSS-first `@theme`)
- **Scheduling**: System `cron` is the ticker; the app manages a section of the user's crontab and reads run logs. `croner` is used only for cron-string compilation and `nextRun` calculations.
- **Run capture**: A bash wrapper script invoked by every cron line (and by every manual "run now") writes one meta JSON + one stdout file per run.
- **Persistence**: Plain JSON files under Electron's `userData/data/`; user-owned executable scripts under `userData/agents/`.
- **Theming**: Reads Omarchy's `~/.config/omarchy/current/theme/colors.toml` at runtime, with a watcher
- **Tests**: Vitest (unit tests live next to the code: `*.test.ts`)
- **Runtime requirements**: `crontab` (Linux: `cronie` or similar; macOS ships it) and `python3` on PATH.

## Features

| Feature | Description |
|---------|-------------|
| [scheduler-engine](features/scheduler-engine/INDEX.md) | Cron-backed scheduler: managed crontab section, wrapper script, missed-run detection, manual re-run |
| [theme-system](features/theme-system/INDEX.md) | Live Omarchy theme load + watch, exposed to the renderer as CSS variables |
| [data-layout](features/data-layout/INDEX.md) | On-disk file layout for schedules, run logs, agent scripts, and the legacy migration |
| [dashboard-ui](features/dashboard-ui/INDEX.md) | React dashboard: sectioned agent cards, run buttons, missed-runs banner, inline schedule editor |

## Quick Links

- [Product brief](../PRODUCT.md)
- [Getting started](../README.md)
