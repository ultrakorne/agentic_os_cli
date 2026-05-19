CLI to schedule, run and manage local agents (just scripts in your filesystem)
If you pc is turned off, the agent will run soon after automatically (can turn off by config)

Also include a tui, just run `aos`

<img width="780" height="426" alt="image" src="https://github.com/user-attachments/assets/8257f8b0-f98f-4a81-ad59-0f7a81827281" />


<img width="1095" height="190" alt="image" src="https://github.com/user-attachments/assets/a45674e6-45de-4a0c-bfe1-4c8363422a24" />


## Install

One-liner that grabs the latest release, linux and mac.

```sh
curl -fsSL https://raw.githubusercontent.com/ultrakorne/agentic_os_cli/master/scripts/install-release.sh | bash
```

(For a system-wide path like `/usr/local/bin` replace after the pipe with `... | sudo -E env BIN_DIR=/usr/local/bin bash`.)

After reinstalling to a different path, run `aos refresh` to rebuild the crontab line against the new binary location.

### macOS

`~/.local/bin` is not a macOS convention; many users prefer
`/usr/local/bin` (Intel Homebrew) or `/opt/homebrew/bin` (Apple Silicon).
Either works — pass `BIN_DIR` to pick one. The crontab line uses
the absolute path either way.

## Build from source

Requires Go on PATH (see `go.mod` for the version).

```
scripts/install.sh
```

## Quick start

```
aos version              # print version, commit, and build date
aos init ~/.aos          # create the aos home, write wrapper.sh, sync crontab
aos refresh              # rescan agents and reconcile the managed crontab block
aos tick                 # one scheduler tick (cron invokes this automatically)
aos list                 # list every agent with its schedule and description
aos describe <id>        # show one agent's full record (or set its description)
aos schedule <id> ...    # set or clear an agent's schedule, then refresh cron
aos run <id>             # fire a manual run; prints the Run stub and estimate
aos run <id> --wait      # same, then block until done and print the agent's .out
aos runs                 # list recent runs (sorted newest first)
aos runs <run-id>        # show one run's record with the captured .out inline
aos uninstall            # remove wrapper, managed crontab block, and config
```

Add `--json` to any command for a machine-readable payload instead of the
human one-liner. See [docs/COMMANDS.md](docs/COMMANDS.md) for the full
reference (every flag, every output shape, the sidecar JSON contract).

### Scheduling syntax

```
aos schedule my-agent --every-hours 3 --minute 0          # every 3h on the hour
aos schedule my-agent --hour 9 --minute 0 --days mon-fri  # weekdays at 09:00
aos schedule my-agent --hour 9 --minute 0 --days mon,wed  # specific days
aos schedule my-agent --off                               # clear the schedule
```

## Where things live

- Binary: `~/.local/bin/aos` (or `$AOS_INSTALL_DIR/aos`)
- Config: `~/.config/aos/config.toml` (points at the aos home)
- Aos home: wherever you passed to `aos init` (e.g. `~/.aos`) — contains
  `agents/`, `runs/`, `wrapper.sh`, `tick.log`
- Crontab: managed block in your user crontab, bracketed by `# BEGIN
  agentic_os` / `# END agentic_os` markers
