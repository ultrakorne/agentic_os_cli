# aos CLI Documentation

`aos` is a Go CLI that installs and manages the agentic_os runtime on a user's machine: it writes `wrapper.sh` into an `aos_home`, reconciles a platform-native scheduler (launchd LaunchAgents on macOS, systemd-user timer+service units on Linux) against discovered agent scripts, ticks the scheduler to detect missed runs and sweep stale records, and offers an interactive TUI to browse and trigger agents.

## Tech Stack

- **Language**: Go (module path `github.com/ultrakorne/aos_cli`)
- **CLI framework**: [`spf13/cobra`](https://github.com/spf13/cobra) for subcommands and flag parsing
- **TUI stack**: `charm.land/bubbletea/v2`, `charm.land/bubbles/v2` (list, viewport, help, key, table, textinput, textarea, progress, spinner), `charm.land/lipgloss/v2` for styled human output
- **Persistence**: plain files under `<aos_home>/`:
  - `agents/<id>.{sh,bash,zsh,...}` — user-provided agent scripts
  - `agents/<id>.meta.json` — optional sidecar holding schedule + description
  - `runs/<run-id>.{json,out}` — per-run record + captured stdout/stderr
  - `wrapper.sh` — embedded shell wrapper executed by the platform scheduler and by `aos run`
  - `tick.log` — append-only tail of every `aos tick` invocation
- **Config**: `~/.config/aos/config.toml` (TOML via `pelletier/go-toml/v2`)
- **Scheduler backends** (live OUTSIDE `aos_home`, in OS-owned namespaces):
  - macOS: `~/Library/LaunchAgents/com.agenticos.*.plist` — encoded via [`howett.net/plist`](https://github.com/DHowett/go-plist), loaded into the per-user GUI domain (`gui/$UID`) so jobs inherit Keychain access
  - Linux: `~/.config/systemd/user/agentic-os-*.{service,timer}` — `Persistent=true` on timers gives native make-up-on-wake; requires linger for headless hosts
- **Filesystem watch**: `fsnotify/fsnotify` for the TUI's live updates

## Features

| Feature | Description |
|---------|-------------|
| [lifecycle](features/lifecycle/INDEX.md) | Install/uninstall and home-path management (`init`, `home`, `uninstall`) |
| [agent-management](features/agent-management/INDEX.md) | Scan agent scripts and edit their sidecar metadata (`list`, `describe`, `schedule`) |
| [run-execution](features/run-execution/INDEX.md) | Fire manual runs and read run records (`run`, `runs`, `--wait`) |
| [scheduler](features/scheduler/INDEX.md) | Platform-backend reconciliation, missed-run detection, stale-running sweep (`refresh`, `tick`) |
| [interactive-tui](features/interactive-tui/INDEX.md) | Bubble Tea dashboard with per-agent details popup (`start`, default verb) |

## Quick Links

- [CONTEXT.md](CONTEXT.md) — Ubiquitous language / project glossary
- [Command reference](COMMANDS.md) — every verb, every flag, every output shape, sidecar contract
