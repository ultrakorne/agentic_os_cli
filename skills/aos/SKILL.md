---
name: aos
description: >
  Use this skill when the user asks you to create, schedule, run, or inspect a
  local agent via the `aos` CLI — anything under `aos list/describe/schedule/
  run/runs/refresh`. Also use it when writing scripts that live under
  `<aos_home>/agents/`
allowed-tools: Bash(aos:*), Bash(jq:*)
---

# aos

`aos` schedules and runs local agent scripts via cron. Agents are plain
executables on disk; a wrapper invoked by cron captures stdout, stderr, and
exit code into a per-run record under `<aos_home>/runs/`.

## Commands

Every verb accepts `--json` — use it. The JSON shape is the contract; the
human view is not. Exit `0` on success, `1` on any failure.

| Verb | Purpose |
|------|---------|
| `aos home` | Print `<aos_home>` |
| `aos list` | Enumerate agents (id, section, schedule, description) |
| `aos describe <id> [text]` | Show one agent's full record; optional 2nd arg rewrites the description |
| `aos schedule <id> ...` | Set or clear schedule (auto-refreshes cron) |
| `aos run <id> [--wait]` | Manual run; `--wait` blocks and prints `.out` after. --wait is blocking |
| `aos runs [<run-id>]` | List recent runs, or show one (captured output inline) |
| `aos refresh` | Rescan agents + reconcile crontab (idempotent) |

`aos init <path>` and `aos uninstall` are installer verbs — only run if the
user asks.

## Where things live

```
<aos_home>/                          # path stored in ~/.config/aos/config.toml
├── agents/
│   ├── ping.sh                      # top-level → section "Agents"
│   ├── ping.meta.json               # optional sidecar
│   └── <section>/<name>.sh          # subdir name becomes the section
└── runs/
    ├── <run-id>.json                # one record per run
    └── <run-id>.out                 # captured stdout+stderr
```

Always resolve the home with `aos home` — don't assume `~/.aos`, users pick it.

## Creating an agent script

Place an executable under `<aos_home>/agents/[<section>/]<name>.<ext>`. The
**id** is the basename without extension; the **section** is the parent dir
(or `"Agents"` at the top level). The scanner enforces `+x` — make the file
executable.

The shebang picks the interpreter, so any language works. A minimal shell
agent — works both under the wrapper and when invoked by hand:

```bash
#!/usr/bin/env bash
set -euo pipefail
# Fall back so the script is still runnable manually outside the wrapper.
DATA_DIR="${AGENTIC_OS_DATA_DIR:-$HOME/.config/agentic-os/data}"
WORKDIR="$DATA_DIR/workspaces/${AGENTIC_OS_AGENT_ID:-$(basename "$0" .sh)}"
mkdir -p "$WORKDIR"
# ... your work here; stdout+stderr is captured to the run's .out file.
```

Use `aos schedule <id> ...` to set a schedule and `aos describe <id> "<text>"`
to set a description — both write the `<script>.meta.json` sidecar for you.

## Env vars exported by wrapper.sh

Every scheduled run is invoked by `wrapper.sh`, which exports:

| Var | Meaning |
|-----|---------|
| `AGENTIC_OS_DATA_DIR` | The `<aos_home>` path. Anchor workspace paths to this — e.g. `"$AGENTIC_OS_DATA_DIR/workspaces/<id>"`. |
| `AGENTIC_OS_AGENT_ID` | This script's id (basename, no extension). |
| `AGENTIC_OS_AGENT_SCRIPT` | Absolute path of the script. |
| `AGENTIC_OS_RUN_ID` | Run id; matches `<run-id>.json`/`<run-id>.out` on disk. |
| `AGENTIC_OS_TRIGGER` | One of `schedule`, `manual`, `catch-up`. |

Cron's `PATH` is minimal; the wrapper prepends `~/.local/bin`, `~/bin`,
`/usr/local/bin`, `/opt/homebrew/bin`. Extend `PATH` yourself for anything
else.

If a script captures output via `$(...)` and exits early, **also capture
stderr** (`$(cmd 2>&1)`) and echo it on failure — otherwise the run record
shows an empty error.

## Scheduling

```
aos schedule <id> --every-hours N --minute M                  # hourly
aos schedule <id> --hour H --minute M --days <list-or-range>  # daily
aos schedule <id> --off                                       # clear
```

- `--every-hours` is `1..12`; `--hour` is `0..23`; `--minute` is `0..59`.
- `--days` accepts a comma list (`mon,wed,fri`) or a single inclusive range
  (`mon-fri`). Week order is `sun..sat`. Reverse ranges and range-plus-comma
  forms are rejected.
- `--off` clears the schedule; cannot be combined with other schedule flags.
- Conflicting flag combinations are rejected outright rather than picking
  a winner.

`aos run <id>` works without a schedule for manual / ad-hoc runs.

## Run records

```json
{
  "id": "...",
  "agentId": "ping",
  "scheduleId": null,
  "trigger": "manual",
  "startedAt": "2026-05-16T13:09:37.072Z",
  "endedAt":   "2026-05-16T13:09:39.103Z",
  "status":    "success",      // running | success | error | missed
  "exitCode":  0,
  "output":    "",             // only populated on single-run reads
  "outputPath": "<run-id>.out"
}
```

Canonical way to read captured stdout+stderr:

```sh
aos runs <run-id> --json | jq -r .output
```

## Catch-up

`aos tick` runs on the cron cadence (`tick_interval` in
`~/.config/aos/config.toml`, default 10m). It records missed slots as runs
with `status: "missed"` and, unless `catchup_enabled = false`, spawns a
catch-up for any agent whose **latest** run on disk is `missed`. Catch-up
runs carry `trigger: "catch-up"` and the missed slot's RFC3339 timestamp
as `scheduleId`. A failed catch-up does not auto-retry.

Only one `miss-*` record per agent ever exists at once — multi-slot outages
collapse to one entry.
