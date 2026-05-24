# `aos` command reference

Every command accepts a persistent `--json` flag (defined on `rootCmd`) for
machine-readable output. Without it, commands print a styled human view
(tables for listings, key/value blocks for single records) rendered with
[lipgloss](https://github.com/charmbracelet/lipgloss). Lipgloss auto-detects
the terminal's color profile and strips styling when stdout isn't a TTY, so
piping or redirecting these commands still produces clean text. Exit codes
follow Unix convention: `0` on success, `1` on any error (CLI flag misuse,
missing agent, sidecar write failure, etc.).

The JSON shape is the contract for clients and agents — it's preserved
across cosmetic changes to the human output. Two helpers in the CLI keep
both surfaces consistent: every `--json` branch funnels through `printJSON`
(two-space indent), and every styled summary uses `printKV` / `newTable`.

## Verbs at a glance

| Command | Role |
|---------|------|
| [`aos init <path>`](#aos-init) | Create the aos home, write `wrapper.sh`, sync platform backend |
| [`aos home`](#aos-home) | Print the configured `aos_home` path |
| [`aos refresh`](#aos-refresh) | Rescan agents and reconcile the platform backend (launchd / systemd-user) |
| [`aos tick`](#aos-tick) | One scheduler tick (the platform backend invokes this on the configured `tick_interval`, default every hour) |
| [`aos list`](#aos-list) | Enumerate every agent with section, schedule summary, description |
| [`aos describe <id> [text]`](#aos-describe) | Show one agent's full record; optionally rewrite its description |
| [`aos schedule <id> ...`](#aos-schedule) | Set or clear an agent's schedule; auto-refreshes the backend |
| [`aos run <id>`](#aos-run) | Fire a manual run; prints the `Run` stub (optional `--wait` blocks until done and prints `.out`) |
| [`aos runs [run-id]`](#aos-runs) | List recent runs, or show one by id (single-run prints the captured .out inline) |
| [`aos uninstall`](#aos-uninstall) | Remove wrapper, platform backend entries, tick.log, and config |

---

## `aos init`

```
aos init <path>
aos init <path> --json
```

Creates the aos home directory (`<path>`), writes `wrapper.sh` and seed
`agents/` + `runs/` subdirectories, stores `<path>` in
`~/.config/aos/config.toml`, and runs a `refresh` to install the platform-native
scheduler entries (launchd LaunchAgents under `~/Library/LaunchAgents/com.agenticos.*`
on macOS, systemd-user `.timer`+`.service` pairs under `~/.config/systemd/user/agentic-os-*`
on Linux). If a previous home was configured and points elsewhere, contents
are relocated to the new path (rename when possible, copy+remove across
filesystems).

The config file is written with every tunable populated at its default so
the available knobs are visible without reading docs:

```toml
aos_home = '/home/you/aos_home'
runs_hard_cap = 2000
tick_interval = "1h"
```

Re-running `aos init` preserves user-set values for `runs_hard_cap` and
`tick_interval`; only `aos_home` is updated to the new path. `tick_interval`
is a Go duration string and must be at least 1 minute (`"1m"`, `"30m"`,
`"1h"`, `"6h"`, …). Anything sub-minute or unparseable is rejected:
`aos refresh` and `aos tick` log the parse error to stderr and fall back
to the default `"1h"`. Edit the value and run `aos refresh` to reschedule
the platform backend's tick entry.

On Linux, after the refresh, init inspects the linger probe result. When
linger is off on a headless session (`XDG_SESSION_TYPE` empty or `"tty"`)
and the operator is at a TTY, init prints a warning and offers to run
`sudo loginctl enable-linger $USER` interactively. Under `--json` the
prompt is skipped; the state is still in `refresh.lingerState`.

**Human output** (styled key/value block):

```
aos init
mode     fresh
home     /home/ultra/Developer/aos_home
wrapper  wrote
— refresh —
agents         2
scheduled      1
issues         0
backend        managed:(wrote=2 unchanged=0)
wrapper        ok
python3        ok
backendHealth  ok
log            untouched
runs           untouched
```

**JSON output:**

```json
{
  "mode": "fresh",
  "home": "/home/ultra/Developer/aos_home",
  "wrapper": "wrote",
  "refresh": { "agents": 2, "scheduled": 1, "backend": { "state": "managed", "wrote": 2 }, ... }
}
```

`mode` is one of `fresh | same | moved | repointed`. `wrapper` is `wrote | same`.

## `aos home`

```
aos home
aos home --json
```

Prints the absolute `aos_home` path on stdout — the human form is the raw
path (no styling) so existing `$(aos home)/runs` patterns keep working.
With `--json`, prints `{"home": "<path>"}`. Exits non-zero if `aos init`
hasn't run yet.

## `aos refresh`

```
aos refresh [--json]
```

Rescans `<aos_home>/agents/`, records any newly-detected missed runs into
`<aos_home>/runs/` (see `aos tick` and `aos runs` for the miss model), calls
`backend.Sync` to reconcile the platform-native scheduler (write / update /
remove plists on macOS or `.timer`+`.service` pairs on Linux; bootstrap or
`enable --now` each), trims `tick.log` if it's too big, and sweeps `runs/`
down to `runs_hard_cap`. Reconciliation is idempotent — running twice in
a row is a no-op (`backend.wrote=0`, `backend.unchanged=N`).

**Human output** (styled key/value block):
```
aos refresh
agents         2
scheduled      1
issues         0
backend        managed:(wrote=0 unchanged=2)
wrapper        ok
python3        ok
backendHealth  ok
linger         ok
log            untouched
runs           untouched
```

Health fields (`wrapper`, `python3`, `backendHealth`, `linger`) are colored
green/yellow/red so a degraded install (missing wrapper, no python3, linger
off on a headless host, …) stands out without reading every line.

`linger` appears only on Linux.

**JSON output:**
```json
{
  "agents": 2,
  "scheduled": 1,
  "issues": 0,
  "backend": {
    "state": "managed",
    "wrote": 0,
    "unchanged": 2,
    "removed": 0
  },
  "wrapper": "ok",
  "python3": "ok",
  "backendHealth": "ok",
  "lingerState": "ok",
  "log": { "trimmed": false },
  "runs": { "deleted": 0 }
}
```

`backend.state` values: `managed | drift | empty | skipped`. When skipped,
`backend.reasons` carries the cause(s) (`no-wrapper`, `no-python3`, an
underlying error). `backend.failed` is omitted when empty; otherwise each
entry is `"<agent-id>: <reason>"`.

`lingerState` is omitted on macOS.

## `aos tick`

```
aos tick
aos tick --json
```

Invoked by the platform backend's tick entry (a `com.agenticos.__tick__`
LaunchAgent on macOS or the `agentic-os-tick.timer` user timer on Linux)
at the cadence set by `tick_interval` in `config.toml` (default every
hour). Each tick does, in order:

1. **Detect missed slots.** For each scheduled agent, find the most-recent
   uncovered slot ≤ now and persist it as `runs/miss-<agent>-<expectedAt>.json`
   with `status:"missed"`. The platform backend's native make-up-on-wake
   (implicit on launchd; `Persistent=true` on systemd timers) handles
   actually re-firing the slot — aos does not dispatch a follow-up run.
2. **Sweep stale running records.** Any `Run` with `status="running"` and
   `startedAt` older than 1 hour is rewritten to `status="error"`,
   `error="no completion record"`, `exitCode=1`. Covers wrappers killed
   mid-run by a backend reload, OOM, or power loss.
3. **Probe backend drift.** Call `backend.State(spec)` and surface the
   result string.
4. **Log a summary** to `<aos_home>/tick.log`.

The default stdout form mirrors the log line; `--json` emits a `TickOutcome`
record:

```json
{
  "timestamp": "2026-05-24T13:00:00Z",
  "scripts": 2,
  "scheduled": 1,
  "missed": 0,
  "staleResolved": 0,
  "backend": "managed"
}
```

The `missed` field counts miss records **newly written this tick**, not
currently outstanding — most ticks emit 0. When a newer uncovered slot is
detected for an agent that already has a miss record, the older record is
replaced; only one `miss-*` file per agent exists on disk at any time. The
deliberate granularity loss (multi-slot outages collapse to one entry) lets
the dashboard surface "agents currently behind" as a one-row-per-agent
banner that auto-resolves on the next real run.

The `staleResolved` field counts `running`→`error` rewrites this tick.

The `backend` field is one of `managed | drift | empty | error(<msg>)`.

The `tick.log` line format is independent of `--json` — it's always:

```
[tick] 2026-05-24T13:00:00Z scripts=2 scheduled=1 missed=0 staleResolved=0 backend=managed
```

## `aos list`

```
aos list [--json]
```

Enumerates every agent visible under `<aos_home>/agents/`. Top-level scripts
fall under section `"Agents"`; first-level subdirectory names become section
names. Duplicate ids are dropped (first-wins) and surfaced as issues.

**Human output** is a styled lipgloss table:
```
╭───────────────┬───────────┬────────────────┬──────────┬──────────────────╮
│ ID            │ SECTION   │ SCHEDULE       │ WARNINGS │ DESCRIPTION      │
├───────────────┼───────────┼────────────────┼──────────┼──────────────────┤
│ daily_planner │ Assistant │ -              │ -        │ What did I do... │
│ ping          │ Agents    │ weekdays 23:00 │ -        │ -                │
╰───────────────┴───────────┴────────────────┴──────────┴──────────────────╯
```

The `SCHEDULE` column collapses three common day-of-week sets for
readability: the full week renders as `everyday`, `mon..fri` as `weekdays`,
and `sat,sun` as `weekends`. Other combinations fall through to a literal
comma list (e.g. `mon,wed,fri`). The collapse is **human-only** — the JSON
`days` array is always the explicit list.

Warnings are colored yellow when non-empty. Issues print to stderr after
the table.

**JSON output:**
```json
{
  "agents": [
    {
      "id": "ping",
      "section": "Agents",
      "scriptPath": "/.../agents/ping.sh",
      "schedule": { "kind": "daily", "days": ["mon", "tue", "wed", "thu", "fri"], "hour": 23, "minute": 0 },
      "scheduledAt": "2026-05-15T20:50:04.341Z"
    }
  ],
  "issues": []
}
```

Optional fields (`schedule`, `scheduledAt`, `description`, `title`, `warnings`)
are **omitted** when unset, not set to `null`. There is no `cron` field on
the agent record — the structured `schedule` is the contract; the per-platform
file format (launchd `StartCalendarInterval` / systemd `OnCalendar`) is an
internal rendering detail of the backend.

## `aos describe`

```
aos describe <id>              # read: print the agent's record
aos describe <id> [--json]
aos describe <id> "<text>"     # write: set the description, then print the record
aos describe <id> ""           # clear the description
```

Returns the **full agent record** (same shape as a single item in
`aos list --json`), not just the description string. With a second positional
argument, writes the description before printing — empty string clears.

**Human output** (styled key/value block with a banner):
```
aos describe ping
section      Agents
script       /.../agents/ping.sh
schedule     weekdays 23:00
scheduledAt  2026-05-15T20:50:04.341Z
description  -
```

**JSON output:** same per-agent shape as `aos list --json` items.

The write form does not trigger a refresh — descriptions don't affect the
platform backend.

## `aos schedule`

```
aos schedule <id> --every-hours N --minute M               # hourly
aos schedule <id> --hour H --minute M --days <list-or-range>  # daily
aos schedule <id> --off                                    # clear
```

Sets or clears an agent's schedule, then runs `refresh` in-process so the
platform backend reflects the change immediately. The schedule **kind is
inferred from the flags** you pass — there is no `--kind` flag.

| Flag | Used by | Notes |
|------|---------|-------|
| `--every-hours N` | hourly | `1..12`. Required for hourly. |
| `--hour H` | daily | `0..23`. Required for daily. |
| `--minute M` | both | `0..59`. Required. |
| `--days <list-or-range>` | daily | Comma list (`mon,wed,fri`) or single inclusive range (`mon-fri`). |
| `--off` | either | Clears the schedule. Cannot be combined with other schedule flags. |

Conflicting flag combinations (`--every-hours` with `--hour` / `--days`, or
`--off` with anything else) are rejected outright rather than picking a
winner.

**Days input forms:**
- `mon,tue,wed,thu,fri` — comma list
- `mon-fri` — inclusive range (week order is `sun..sat`)

Reverse ranges (`fri-mon`) and range-plus-comma forms (`mon-fri,sun`) are
rejected.

**Human output** (styled key/value block plus the refresh summary):
```
aos schedule ping
kind         daily
days         mon,tue,wed,thu,fri
hour         9
minute       0
scheduledAt  2026-05-16T...
— refresh —
agents     2
scheduled  2
…
```

For `--off`:
```
aos schedule ping
schedule  cleared
— refresh —
…
```

**JSON output:**
```json
{
  "id": "ping",
  "schedule": { "kind": "daily", "days": ["mon", ...], "hour": 9, "minute": 0 },
  "scheduledAt": "2026-05-16T...",
  "refresh": { "agents": 2, "scheduled": 2, "backend": { "state": "managed", "wrote": 1 }, ... }
}
```

When the post-write refresh fails, the schedule write still succeeds and the
failure is reported as `"refresh": { "error": "..." }` rather than aborting
the command. The human path prints a `warn:` line to stderr in the same
case.

## `aos run`

```
aos run <id>                  # spawn a manual run; prints the Run stub and exits
aos run <id> --json
aos run <id> --wait           # spawn, then block until done; prints .out on stdout
aos run <id> --wait --json    # spawn, print stub JSON, block, then append .out
```

Looks up the agent by id, estimates duration from the newest successful runs
for that agent (up to 10), mints a run id (`<unix-ms>-<rand4>`), spawns
`wrapper.sh` detached (`setsid`) with `AGENTIC_OS_TRIGGER=manual` and the
explicit run id as the wrapper's 4th argv, then prints a `Run` stub. The
wrapper writes the final record under `<aos_home>/runs/<run-id>.json` once
the script exits — poll for it (or watch the file) to see the result.

The estimate uses only `success` records with parseable `startedAt` and
`endedAt` — `error`, `running`, and `missed` runs are skipped so a
fast-failing script doesn't drag the ETA below the typical successful
runtime. If the agent has no successful history, human output prints
`estimate  none` and JSON prints `"estimate": -1`. Otherwise JSON
`estimate` is the average elapsed time in milliseconds.

Errors exit non-zero with the message on stderr: missing agent, wrapper
absent / not executable.

**Human output** (styled key/value block; `status` colored amber):
```
aos run ping
run        1778936977-a93c
status     running
estimate   2.0s
startedAt  2026-05-16 13:09:37
```

**JSON output:** the same shape as `aos runs <run-id> --json` with
`status: "running"`, `endedAt: null`, `exitCode: null`, `output: ""`, plus
`estimate` in milliseconds (`-1` when unknown). The persisted run record the
wrapper later writes does not include `estimate`.

### `--wait`

`aos run <id> --wait` keeps the same stub-first behavior — the stub still
prints to stdout immediately after `wrapper.sh` is spawned — and then blocks
until the wrapper writes a terminal record (`success` / `error`).

- **Progress UI on stderr.** While waiting, a Bubble Tea progress bar (when
  an estimate exists) or spinner (when it doesn't) renders on **stderr** so
  the run summary on stdout stays untouched. Piping stdout into another tool
  is unaffected.
- **`.out` on stdout after the wait.** Once the wrapper finishes, the raw
  bytes of `<aos_home>/runs/<run-id>.out` are written to stdout, so:
    - Human: `stub block → progress on stderr → .out bytes` on stdout
    - `--json`: `stub JSON → progress on stderr → .out bytes` appended to
      stdout. Stdout intentionally ends up as "JSON then output"; consumers
      that want only the structured record should drop `--wait`.
- **Ctrl+C while waiting** stops the polling loop, prints a one-line message
  to stderr citing the run id ("run is still executing in the background"),
  and exits non-zero. The wrapper was spawned detached, so the agent run
  itself is unaffected — `aos runs <run-id>` will surface the result once
  it eventually finishes.
- **Failed runs** print `.out` first (so stderr emitted by the script is
  preserved for the operator), then `aos run --wait` returns a non-zero exit
  code carrying the underlying status code (`run <id> exited with code N`).

## `aos runs`

```
aos runs                            # list recent runs, newest first
aos runs --agent <id>               # filter by agent id
aos runs --limit N                  # cap result size (default 25; 0 = no limit)
aos runs --json
aos runs <run-id>                   # show one run's record + captured .out
aos runs <run-id> --json
```

Reads `<aos_home>/runs/<run-id>.{json,out}` and emits the same shape `aos run`
writes. Sort is by `startedAt` descending — ISO-8601 timestamps sort
chronologically as strings.

Malformed `<run-id>.json` files are silently skipped (the wrapper writes
atomically via `mv`, but a concurrent reader can still hit a partial state in
rare cases).

**Human list output** (styled lipgloss table; `STATUS` colored per state).
A muted `showing N of M runs` line precedes the table so it's obvious when
`--limit` is hiding records:
```
showing 2 of 14 runs
╭──────────────────┬───────┬─────────┬──────────┬─────────────────────┬─────────╮
│ RUN-ID           │ AGENT │ STATUS  │ TRIGGER  │ STARTED             │ ELAPSED │
├──────────────────┼───────┼─────────┼──────────┼─────────────────────┼─────────┤
│ 1778936977-a93c  │ ping  │ success │ manual   │ 2026-05-16 13:09:37 │ 2.031s  │
│ 1778878800-1b40  │ ping  │ success │ schedule │ 2026-05-15 21:00:00 │ 2.029s  │
╰──────────────────┴───────┴─────────┴──────────┴─────────────────────┴─────────╯
```

`STATUS` is colored amber (running), green (success), red (error), or
yellow (missed) — the underlying exit code lives in the single-run view, since
the colored status already conveys the pass/fail signal at list scale.
`ELAPSED` is `...` while the run is still in flight and `—` for `missed`
records (they never ran).

### Missed runs

A run with `status: "missed"` is a scheduled slot the wrapper never fired —
`aos tick` and `aos refresh` persist these into `<aos_home>/runs/` so they
appear in the timeline alongside real runs. Shape:

- `id`: `miss-<agentId>-<expectedAt>` (deterministic, ':' replaced with '-'
  for filesystem portability)
- `startedAt`: the expected slot (RFC3339), not when the miss was recorded
- `endedAt`, `exitCode`, `outputPath`, `error`: all `null`
- `trigger`: `"schedule"`
- `output`: `""` — there is no `.out` file for a missed run, so the
  single-run view renders no `output` section

Only **one** miss record per agent exists on disk at any time. When a newer
uncovered slot is detected, the previous miss for that agent is deleted and
replaced — multi-slot outages deliberately collapse to one entry so the
dashboard's "agents currently behind" banner is one row per agent. Aos does
**not** dispatch a follow-up run for the miss; the platform backend's native
make-up-on-wake re-fires the schedule on its own (launchd's behavior for
`StartCalendarInterval` jobs, systemd's `Persistent=true` on `.timer` units).

**Human single-run output** (styled key/value block with the run-id as banner,
followed by the captured stdout/stderr from the `.out` file as a labeled
section):
```
aos runs 1778936977-a93c
agent       ping
status      success
trigger     manual
startedAt   2026-05-16 13:09:37
endedAt     2026-05-16 13:09:39
elapsed     2.031s
exit        0
outputPath  1778936977-a93c.out

output
ping at 2026-05-16T13:09:39Z
```

The `output` section is omitted when the run produced no output yet (still
running, or finished without writing to stdout/stderr). To pipe just the raw
output, use `aos runs <id> --json | jq -r .output`.

**JSON list output:**
```json
{
  "runs": [
    {
      "id": "1778936977-a93c",
      "agentId": "ping",
      "trigger": "manual",
      "startedAt": "2026-05-16T13:09:37.072Z",
      "endedAt": "2026-05-16T13:09:39.103Z",
      "status": "success",
      "output": "",
      "error": null,
      "exitCode": 0,
      "outputPath": "1778936977-a93c.out"
    }
  ]
}
```

The `Run` record has no `scheduleId` field — the cron-era concept of "missed
slot id attached to a catch-up run" is gone, since catch-ups themselves are
gone (native makeup replaces them). `trigger` is one of `schedule` or
`manual`; there is no `catch-up` value.

**JSON single-run output:** the inner record only (no `runs` wrapper), with
the `output` field populated from the run's `.out` file (empty string when
nothing was captured). The list output above leaves `output` empty so a
listing of N runs doesn't balloon with full transcripts.

## `aos uninstall`

```
aos uninstall
aos uninstall --json
```

Removes the installed `wrapper.sh`, the `tick.log`, the platform backend
entries (via `backend.Remove()` — boots-out every LaunchAgent in the
`com.agenticos.*` namespace on macOS, or disables and unlinks every
`.timer`+`.service` pair in the `agentic-os-*` namespace on Linux), and
`~/.config/aos/config.toml`. The `agents/` and `runs/` directories are
**left untouched** — they contain user data.

**Human output** (styled key/value block; each field colored green when
`removed`, yellow when `skipped:*`, plain otherwise):
```
aos uninstall
wrapper  removed
tickLog  removed
backend  removed
config   removed
```

**JSON output:**
```json
{
  "wrapper": "removed",
  "tickLog": "removed",
  "backend": "removed",
  "config": "removed"
}
```

Each field is one of `removed | absent | skipped:<reason>`. When the config
couldn't be loaded or `aos_home` was unset, `backend` reads
`skipped:no-config`; backend-side errors are reported as
`skipped:<sanitized-error>` (no spaces, max 60 chars).

---

## Sidecar contract

Every agent script can have an optional sidecar at `<script-path>.meta.json`
(e.g. `agents/ping.sh` → `agents/ping.meta.json`). The sidecar is what
`aos schedule` and `aos describe` write. The Electron renderer writes the
same shape from its own meta store.

```json
{
  "schedule": { "kind": "hourly", "everyHours": 3, "minute": 0 },
  "scheduledAt": "2026-05-16T12:00:00Z",
  "title": "Ping",
  "description": "Healthcheck"
}
```

Rules:

- **All fields are optional.** A sidecar with only `description` is valid.
- **`schedule.kind`** is either `"hourly"` or `"daily"`. Hourly carries
  `everyHours` (1..12) and `minute` (0..59). Daily carries `days` (subset of
  `sun..sat`), `hour` (0..23), and `minute` (0..59).
- **`scheduledAt`** is bumped to the current UTC time only when the
  `schedule` spec **actually changes** (day list reordering doesn't count —
  days are compared as sets). It is **not** bumped when other fields like
  `description` change.
- **Atomic writes.** Sidecars are written via temp+rename so a crash never
  leaves a half-written file. An update that would produce an empty meta
  (`{}`) deletes the file instead of leaving a stub.
- **Section is not stored.** Section is recovered from the script's parent
  directory at scan time (top-level → `"Agents"`; first-level subdir →
  subdir name).

The shared types are mirrored on both sides: Go
(`internal/scheduler/schedspec/spec.go` — `ScheduleSpec`, `Weekday`;
`internal/scheduler/spec.go` — `AgentMeta`) and TypeScript
(`src/shared/scheduler.ts`). Keeping these in lockstep is part of the
contract; tests on either side will catch drift in compilation behavior, not
in the JSON shape itself.
