# aos

Go CLI that installs and manages the agentic_os runtime: writes `wrapper.sh`
into your aos home, reconciles your user crontab from agent definitions, and
ticks the scheduler.

## Install

Requires Go on PATH.

```
scripts/install.sh
```

Builds a stripped prod binary (`-trimpath -ldflags="-s -w"`) and drops it at
`~/.local/bin/aos`. Override the destination with `AOS_INSTALL_DIR`:

```
AOS_INSTALL_DIR=/usr/local/bin scripts/install.sh
```

If `~/.local/bin` is not on your interactive `PATH`, the script prints the
one-line `export` to add to your shell rc. Cron is unaffected — the managed
crontab block bakes in the absolute path of the binary at refresh time, so
cron's minimal PATH never needs to include the install dir.

After reinstalling to a different path, run `aos refresh` to rebuild the
crontab line against the new binary location.

### macOS

`~/.local/bin` is not a macOS convention; many users prefer
`/usr/local/bin` (Intel Homebrew) or `/opt/homebrew/bin` (Apple Silicon).
Either works — pass `AOS_INSTALL_DIR` to pick one. The crontab line uses
the absolute path either way.

## Quick start

```
aos init ~/.aos          # create the aos home, write wrapper.sh, sync crontab
aos refresh              # rescan agents and reconcile the managed crontab block
aos tick                 # one scheduler tick (cron invokes this automatically)
aos uninstall            # remove wrapper, managed crontab block, and config
```

## Where things live

- Binary: `~/.local/bin/aos` (or `$AOS_INSTALL_DIR/aos`)
- Config: `~/.config/aos/config.toml` (points at the aos home)
- Aos home: wherever you passed to `aos init` (e.g. `~/.aos`) — contains
  `agents/`, `runs/`, `wrapper.sh`, `tick.log`
- Crontab: managed block in your user crontab, bracketed by `# BEGIN
  agentic_os` / `# END agentic_os` markers
