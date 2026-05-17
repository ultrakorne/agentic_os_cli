# aos CLI Documentation

`aos` is a Go CLI that installs and manages the agentic_os runtime on a user's machine: it writes `wrapper.sh` into an `aos_home`, reconciles a managed block of the user's crontab against discovered agent scripts, ticks the scheduler to detect missed runs, and offers an interactive TUI to browse and trigger agents.

## Tech Stack

- **Language**: Go (module path `github.com/ultrakorne/aos_cli`)
- **CLI framework**: [`spf13/cobra`](https://github.com/spf13/cobra) for subcommands and flag parsing
- **TUI stack**: `charm.land/bubbletea/v2`, `charm.land/bubbles/v2` (list, viewport, help, key, table, textinput, textarea, progress, spinner), `charm.land/lipgloss/v2` for styled human output
- **Persistence**: plain files under `<aos_home>/`:
  - `agents/<id>.{sh,bash,zsh,...}` — user-provided agent scripts
  - `agents/<id>.meta.json` — optional sidecar holding schedule + description
  - `runs/<run-id>.{json,out}` — per-run record + captured stdout/stderr
  - `wrapper.sh` — embedded shell wrapper executed by cron and by `aos run`
  - `tick.log` — append-only tail of every `aos tick` invocation
- **Config**: `~/.config/aos/config.toml` (TOML via `pelletier/go-toml/v2`)
- **Cron integration**: shells out to the user's `crontab(1)`; the managed section is bracketed by `# BEGIN agentic_os` / `# END agentic_os` markers
- **Cron parser**: `robfig/cron/v3` (used only to walk slots when detecting missed runs)
- **Filesystem watch**: `fsnotify/fsnotify` for the TUI's live updates

## Features

| Feature | Description |
|---------|-------------|
| [lifecycle](features/lifecycle/INDEX.md) | Install/uninstall and home-path management (`init`, `home`, `uninstall`) |
| [agent-management](features/agent-management/INDEX.md) | Scan agent scripts and edit their sidecar metadata (`list`, `describe`, `schedule`) |
| [run-execution](features/run-execution/INDEX.md) | Fire manual runs and read run records (`run`, `runs`, `--wait`) |
| [scheduler](features/scheduler/INDEX.md) | Cron reconciliation, missed-run detection, catch-up dispatch (`refresh`, `tick`) |
| [interactive-tui](features/interactive-tui/INDEX.md) | Bubble Tea dashboard with per-agent details popup (`start`, default verb) |

## Quick Links

- [CONTEXT.md](CONTEXT.md) — Ubiquitous language / project glossary
- [Command reference](COMMANDS.md) — every verb, every flag, every output shape, sidecar contract
