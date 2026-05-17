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
| [`aos init <path>`](#aos-init) | Create the aos home, write `wrapper.sh`, sync crontab |
| [`aos home`](#aos-home) | Print the configured `aos_home` path |
| [`aos refresh`](#aos-refresh) | Rescan agents and rewrite the managed crontab block |
| [`aos tick`](#aos-tick) | One scheduler tick (cron invokes this every 10 min) |
| [`aos list`](#aos-list) | Enumerate every agent with section, schedule summary, description |
| [`aos describe <id> [text]`](#aos-describe) | Show one agent's full record; optionally rewrite its description |
| [`aos schedule <id> ...`](#aos-schedule) | Set or clear an agent's schedule; auto-refreshes cron |
| [`aos run <id>`](#aos-run) | Fire a manual run; prints the `JobRun` stub (optional `--wait` blocks until done and prints `.out`) |
| [`aos runs [run-id]`](#aos-runs) | List recent runs, or show one by id (single-run prints the captured .out inline) |
| [`aos uninstall`](#aos-uninstall) | Remove wrapper, managed crontab block, and config |

---

## `aos init`

```
aos init <path>
aos init <path> --json
```

Creates the aos home directory (`<path>`), writes `wrapper.sh` and seed
`agents/` + `runs/` subdirectories, stores `<path>` in
`~/.config/aos/config.toml`, and runs a `refresh` to install the managed
crontab block. If a previous home was configured and points elsewhere,
contents are relocated to the new path (rename when possible,
copy+remove across filesystems).

**Human output** (styled key/value block):

```
aos init
mode     fresh
home     /home/ultra/Developer/aos_home
wrapper  wrote
— refresh —
agents     2
scheduled  1
…
```

**JSON output:**

```json
{
  "mode": "fresh",
  "home": "/home/ultra/Developer/aos_home",
  "wrapper": "wrote",
  "refresh": { "agents": 2, "scheduled": 1, ... }
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

Rescans `<aos_home>/agents/`, recomputes the managed crontab section from each
agent's `<id>.meta.json` sidecar, records any newly-detected missed runs into
`<aos_home>/runs/` (see `aos tick` and `aos runs` for the miss model), and
trims `tick.log` if it's too big. Reconciliation is idempotent — running
twice in a row is a no-op.

**Human output** (styled key/value block):
```
aos refresh
agents     2
scheduled  1
issues     0
cron       wrote
wrapper    ok
python3    ok
daemon     ok
log        untouched
```

Health fields (`cron`, `wrapper`, `python3`, `daemon`, `log`) are colored
green/yellow/red so a degraded install (missing wrapper, cron daemon down,
etc.) stands out without reading every line.

**JSON output:**
```json
{
  "agents": 2,
  "scheduled": 1,
  "issues": 0,
  "cron": "wrote",
  "wrapper": "ok",
  "python3": "ok",
  "daemon": "ok",
  "log": "untouched"
}
```

Cron field values: `wrote | unchanged | skipped:<reason>`. Reasons stack
(`skipped:no-crontab-bin,no-python3`).

## `aos tick`

```
aos tick
aos tick --json
```

Invoked by cron via the managed `__tick__` line every 10 minutes. Detects
the most-recent uncovered scheduled slot per agent and, when one exists,
persists it as a `runs/miss-<agent>-<expectedAt>.json` record with
`status:"missed"` so the dashboard's run-history view picks it up like any
other run. Appends a one-line summary to `<aos_home>/tick.log`. The default
stdout form mirrors that log line; `--json` emits a `TickSummary` record:

```json
{
  "timestamp": "2026-05-16T13:00:00Z",
  "scripts": 2,
  "scheduled": 1,
  "missed": 0,
  "crontab": "managed"
}
```

The `missed` field counts miss records **newly written this tick**, not
currently outstanding — most ticks emit 0. When a newer uncovered slot is
detected for an agent that already has a miss record, the older record is
replaced; only one `miss-*` file per agent exists on disk at any time. The
deliberate granularity loss (multi-slot outages collapse to one entry) lets
the dashboard surface "agents currently behind" as a one-row-per-agent
banner that auto-resolves on the next real run.

The `tick.log` line format is unchanged regardless of `--json` — it's a
separate concern from stdout.

## `aos list`

```
aos list [--json]
```

Enumerates every agent visible under `<aos_home>/agents/`. Top-level scripts
fall under section `"Agents"`; first-level subdirectory names become section
names. Duplicate ids are dropped (first-wins) and surfaced as issues.

**Human output** is a styled lipgloss table:
```
╭───────────────┬───────────┬────────────────┬──────────┬──────────────────┮
│ ID            │ SECTION   │ SCHEDULE       │ WARNINGS │ DESCRIPTION      │
├───────────────┼───────────┼────────────────┼──────────┼──────────────────┤
│ daily_planner │ assistant │ -              │ -        │ What did I do... │
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
      "cron": "0 23 * * 1,2,3,4,5",
      "scheduledAt": "2026-05-15T20:50:04.341Z"
    }
  ],
  "issues": []
}
```

Optional fields (`schedule`, `cron`, `scheduledAt`, `description`) are
**omitted** when unset, not set to `null`.

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
cron         0 23 * * 1,2,3,4,5
scheduledAt  2026-05-15T20:50:04.341Z
description  -
```

**JSON output:** same per-agent shape as `aos list --json` items.

The write form does not trigger a refresh — descriptions don't affect cron.

## `aos schedule`

```
aos schedule <id> --every-hours N --minute M               # hourly
aos schedule <id> --hour H --minute M --days <list-or-range>  # daily
aos schedule <id> --off                                    # clear
```

Sets or clears an agent's schedule, then runs `refresh` in-process so cron
reflects the change immediately. The schedule **kind is inferred from the
flags** you pass — there is no `--kind` flag.

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
rejected. The compiled cron expression uses cron weekday numbering
(`sun=0..sat=6`).

**Human output** (styled key/value block plus the refresh summary):
```
aos schedule ping
kind         daily
days         mon,tue,wed,thu,fri
hour         9
minute       0
cron         0 9 * * 1,2,3,4,5
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
  "cron": "0 9 * * 1,2,3,4,5",
  "scheduledAt": "2026-05-16T...",
  "refresh": { "agents": 2, "scheduled": 2, ... }
}
```

When the post-write refresh fails, the schedule write still succeeds and the
failure is reported as `"refresh": { "error": "..." }` rather than aborting
the command. The human path prints a `warn:` line to stderr in the same
case.

## `aos run`

```
aos run <id>                  # spawn a manual run; prints the JobRun stub and exits
aos run <id> --json
aos run <id> --wait           # spawn, then block until done; prints .out on stdout
aos run <id> --wait --json    # spawn, print stub JSON, block, then append .out
```

Looks up the agent by id, estimates duration from the newest completed runs
for that agent (up to 10), mints a run id (`<unix>-<pid>-<rand><rand>`),
spawns `wrapper.sh` detached (`setsid`) with `AGENTIC_OS_TRIGGER=manual` and
the explicit run id as the wrapper's 5th argv, then prints a `JobRun` stub.
The wrapper writes the final record under `<aos_home>/runs/<run-id>.json`
once the script exits — poll for it (or watch the file) to see the result.

The estimate uses completed records with parseable `startedAt` and `endedAt`.
If the agent has no completed history, human output prints `estimate  none`
and JSON prints `"estimate": -1`. Otherwise JSON `estimate` is the average
elapsed time in milliseconds.

Errors exit non-zero with the message on stderr: missing agent, wrapper
absent / not executable.

**Human output** (styled key/value block; `status` colored amber):
```
aos run ping
run        1778936977-29334-...
status     running
estimate   2.031s
startedAt  2026-05-16T13:09:37.061Z
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
╭──────────────────────────────────────┬───────┬─────────┬──────────┬─────────────────────┬─────────╮
│ RUN-ID                               │ AGENT │ STATUS  │ TRIGGER  │ STARTED             │ ELAPSED │
├──────────────────────────────────────┼───────┼─────────┼──────────┼─────────────────────┼─────────┤
│ 1778936977-29334-5144069401071970568 │ ping  │ success │ manual   │ 2026-05-16 13:09:37 │ 2.031s  │
│ 1778878800-542403-1886130594         │ ping  │ success │ schedule │ 2026-05-15 21:00:00 │ 2.029s  │
╰──────────────────────────────────────┴───────┴─────────┴──────────┴─────────────────────┴─────────╯
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
dashboard's "agents currently behind" banner is one row per agent.

**Human single-run output** (styled key/value block with the run-id as banner,
followed by the captured stdout/stderr from the `.out` file as a labeled
section):
```
aos runs 1778936977-29334-...
agent       ping
status      success
trigger     manual
startedAt   2026-05-16 13:09:37
endedAt     2026-05-16 13:09:39
elapsed     2.031s
exit        0
outputPath  1778936977-29334-5144069401071970568.out

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
      "id": "1778936977-29334-...",
      "jobId": "ping",
      "scheduleId": null,
      "trigger": "manual",
      "startedAt": "2026-05-16T13:09:37.072Z",
      "endedAt": "2026-05-16T13:09:39.103Z",
      "status": "success",
      "output": "",
      "error": null,
      "exitCode": 0,
      "outputPath": "1778936977-29334-....out"
    }
  ]
}
```

**JSON single-run output:** the inner record only (no `runs` wrapper), with
the `output` field populated from the run's `.out` file (empty string when
nothing was captured). The list output above leaves `output` empty so a
listing of N runs doesn't balloon with full transcripts.

## `aos uninstall`

```
aos uninstall
aos uninstall --json
```

Removes the managed crontab block, deletes the installed `wrapper.sh`, and
removes `~/.config/aos/config.toml`. The `agents/` and `runs/` directories
are **left untouched** — they contain user data.

**Human output** (styled key/value block; each field colored green when
`removed`, yellow when `skipped:*`, plain otherwise):
```
aos uninstall
wrapper  removed
cron     removed
config   removed
kept     (none)
```

**JSON output:**
```json
{
  "wrapper": "removed",
  "cron": "removed",
  "config": "removed",
  "kept": []
}
```

`kept` lists any `agents/`/`runs/` path that wasn't empty and was preserved.

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

The shared types are mirrored on both sides: Go (`internal/scheduler/spec.go`
— `AgentMeta`, `ScheduleSpec`) and TypeScript (`src/shared/scheduler.ts`).
Keeping these in lockstep is part of the contract; tests on either side will
catch drift in compilation behavior, not in the JSON shape itself.
