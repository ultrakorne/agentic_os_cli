# Agentic OS

A Dashboard for your agents
Just uses cron to schedule jobs

<img width="975" height="681" alt="agentic os screenshot" src="https://github.com/user-attachments/assets/a8e7f313-a11a-4e98-95ba-87d66d3f92eb" />
<img width="980" height="727" alt="image" src="https://github.com/user-attachments/assets/d33a82ba-2d0e-45ae-a135-2e5a76735bce" />

## Why?

Not and easy way to rerun / check missed jobs when a machine is not always online
Claude / Codex app have limited scheduling and it's also tight to one vendor.

## Architecture

- A wrapper script that records each run's metadata so missed jobs can be
  identified and re-run later.
- All state lives in the filesystem under an `aos_home` directory.
- The [`aos` CLI](agentic_os_cli/) (Go) owns the runtime: it installs the
  wrapper, manages the cron block, and ticks the scheduler — so scheduled
  agents fire whether or not the dashboard is open.
- The Electron dashboard is a *view*: it reads from `aos_home`, writes
  per-agent `<id>.meta.json` sidecars, and shells out to `aos refresh`
  when state changes. If the CLI is not on PATH, the view blocks with an
  install banner.

## Install

```
cd agentic_os_cli
scripts/install.sh           # builds and installs `aos` to ~/.local/bin
aos init ~/.aos              # create the aos home and seed cron
```

Then run the dashboard:

```
pnpm install
pnpm dev
```

## Where things live

All runtime state lives under the `aos_home` you passed to `aos init`
(e.g. `~/.aos`). The dashboard discovers this path by calling `aos home`.

```
~/.aos/
  agents/                    — your scripts (drop files here)
    ping.sh                  → id "ping", section "Agents"
    <id>.meta.json           → schedule, description, written by the dashboard
  workspaces/                — optional per-agent working dirs
  runs/                      — one <run-id>.json + <run-id>.out per run
  wrapper.sh                 — installed by `aos init`
  tick.log                   — cron-driven scheduler tick log
```

Config:

- `~/.config/aos/config.toml` — points at the aos_home

Back up `aos_home/` to move to another machine.

## Adding an agent

1. Drop an executable shell script into `data/agents/` (top level becomes the
   "Agents" section) or `data/agents/<Section>/` (the folder name becomes the
   section).
2. `chmod +x` the file. Use a real shebang (`#!/usr/bin/env bash`).
3. Open the dashboard — the agent appears. Set a schedule from the inline
   editor or click "Run now" for one-off runs.

Filename without extension = agent id. Ids must be unique across the whole
`agents/` tree. Non-shell agents (Python, Node, etc.) work via a one-line
shim that `exec`s the real interpreter.

## Workspaces (optional)

If an agent needs a working directory — prompt files, state it reads/writes,
caches, anything bigger than a one-liner — drop a folder under
`<aos_home>/workspaces/<name>/` and reference it from the script. Many
agents don't need one (`ping`, anything purely stateless); skip it when
you don't.

## Wrapper environment

Every agent script is invoked by `wrapper.sh`, which exports a small set of
env vars so scripts can write portable paths instead of hard-coding the
platform-specific data root:

| Variable                  | Value                                                 |
| ------------------------- | ----------------------------------------------------- |
| `AGENTIC_OS_DATA_DIR`     | The aos_home root                                     |
| `AGENTIC_OS_AGENT_ID`     | Id of the agent being run                             |
| `AGENTIC_OS_AGENT_SCRIPT` | Absolute path to the script being run                 |
| `AGENTIC_OS_RUN_ID`       | Run id (matches the `runs/<id>.{json,out}` filenames) |
| `AGENTIC_OS_TRIGGER`      | `schedule` or `manual`                                |

So a script can stay identical across Linux and macOS:

```bash
#!/usr/bin/env bash
set -euo pipefail
WORKDIR="${AGENTIC_OS_DATA_DIR:-$HOME/.aos}/workspaces/my_agent"
cd "$WORKDIR"
# ...
```

The fallback after `:-` lets the script also be runnable by hand outside
the wrapper.

`PATH` inside the wrapper is `/usr/local/bin:/opt/homebrew/bin:/usr/bin:/bin:$PATH`.
Cron itself starts with almost no environment — anything beyond `PATH` is
the script's responsibility.

## Run logs

Each run produces two files under `<aos_home>/runs/`:

- `<run-id>.json` — meta (status, exit code, started/ended timestamps, trigger)
- `<run-id>.out` — combined stdout + stderr

The dashboard streams these files via `fs.watch`. The runs directory is
capped at 2000 files (oldest deleted, paired so `.json` + `.out` always
age out together).

## Requirements

- `crontab` on PATH (Linux: `cronie` or equivalent — and the daemon must be
  enabled, e.g. `systemctl enable --now cronie`; macOS ships cron).
- `python3` on PATH (the wrapper uses it to write JSON meta files).

The dashboard surfaces missing requirements in a banner at the top.

## Development

```
pnpm install      # install
pnpm dev          # run app (requires `aos` on PATH)
pnpm test         # vitest
pnpm typecheck    # node + web
pnpm lint
pnpm build:linux  # also: build:mac
```

CLI development lives under `agentic_os_cli/`:

```
cd agentic_os_cli
go test ./...
go build ./...
scripts/install.sh
```

Unit tests live next to source as `*.test.ts` / `*_test.go`.
