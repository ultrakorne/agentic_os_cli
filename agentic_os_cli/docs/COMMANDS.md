# `aos` command reference

Every command accepts a persistent `--json` flag (defined on `rootCmd`) for
machine-readable output. Without it, commands print a single human-readable
line (or a small block) suitable for terminal use. Exit codes follow Unix
convention: `0` on success, `1` on any error (CLI flag misuse, missing agent,
sidecar write failure, etc.).

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
| [`aos uninstall`](#aos-uninstall) | Remove wrapper, managed crontab block, and config |

---

## `aos init`

```
aos init <path>
```

Creates the aos home directory (`<path>`), writes `wrapper.sh` and seed
`agents/` + `runs/` subdirectories, stores `<path>` in
`~/.config/aos/config.toml`, and runs a `refresh` to install the managed
crontab block. If a previous home was configured and points elsewhere,
contents are relocated to the new path (rename when possible,
copy+remove across filesystems).

## `aos home`

```
aos home
```

Prints the absolute `aos_home` path on stdout. Exits non-zero if `aos init`
hasn't run yet. Used by the Electron app to discover where to read from.

## `aos refresh`

```
aos refresh [--json]
```

Rescans `<aos_home>/agents/`, recomputes the managed crontab section from each
agent's `<id>.meta.json` sidecar, rebuilds the misses directory, and trims
`tick.log` if it's too big. Reconciliation is idempotent — running twice in a
row is a no-op.

**Human output** (one line):
```
aos refresh agents=2 scheduled=1 issues=0 cron=wrote wrapper=ok python3=ok daemon=ok log=untouched
```

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
```

Invoked by cron via the managed `__tick__` line every 10 minutes. Detects
missed runs, syncs the misses directory, and appends a one-line summary to
`<aos_home>/tick.log`. Same one-line summary is also written to stdout for
the cron tail. Not commonly run by hand.

## `aos list`

```
aos list [--json]
```

Enumerates every agent visible under `<aos_home>/agents/`. Top-level scripts
fall under section `"Agents"`; first-level subdirectory names become section
names. Duplicate ids are dropped (first-wins) and surfaced as issues.

**Human output** is a tab-separated table:
```
ID             SECTION    SCHEDULE                   DESCRIPTION
daily_planner  assistant  -                          What did I do yesterday...
ping           Agents     mon,tue,wed,thu,fri 23:00  -
```

Issues print to stderr after the table.

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

**Human output** (key/value block):
```
id           ping
section      Agents
script       /.../agents/ping.sh
schedule     mon,tue,wed,thu,fri 23:00
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

**Human output** (one line):
```
aos schedule id=ping kind=daily days=mon,tue,wed,thu,fri hour=9 minute=0 cron="0 9 * * 1,2,3,4,5" scheduledAt=2026-05-16T... | aos refresh agents=2 scheduled=2 issues=0 cron=wrote ...
```

For `--off`:
```
aos schedule id=ping cleared | aos refresh ...
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

## `aos uninstall`

```
aos uninstall
```

Removes the managed crontab block, deletes the installed `wrapper.sh`, and
removes `~/.config/aos/config.toml`. The `agents/` and `runs/` directories
are **left untouched** — they contain user data.

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
