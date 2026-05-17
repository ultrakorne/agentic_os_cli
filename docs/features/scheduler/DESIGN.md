# scheduler — Design

## Overview

Two verbs share one body of logic. `aos refresh` is the *idempotent reconciler*: rescan agents, rewrite the managed crontab block, record newly-detected missed slots, sweep the runs cap, trim `tick.log`. `aos tick` is the *cron-driven heartbeat*: cron invokes it every `tick_interval` (default 10m) via a managed `__tick__` entry; one tick detects misses, fires catch-ups for agents whose latest run was missed, and appends one line to `tick.log`. Together they keep the cron block honest and surface scheduled slots the wrapper missed.

## Components

### `aos refresh`

The full reconciliation pass. Rescans `<aos_home>/agents/`, compiles each scheduled agent's spec into a cron line, rebuilds the managed block (bracketed by `# BEGIN agentic_os` / `# END agentic_os`), and writes the user's crontab only if the result differs. Also records newly-detected miss records into `runs/miss-*.json`, trims `tick.log` if it exceeds the byte cap, and sweeps `runs/` down to `runs_hard_cap`. Reports per-surface health: `cron`, `wrapper`, `python3`, `daemon`, `log`, `runs` — each green/yellow/red in human output.

### `aos tick`

One scheduler cycle, fired by cron. Detects the latest uncovered slot per scheduled agent, persists it as a `miss-*.json` (replacing any older miss record for that agent), and — unless `catchup_enabled = false` — spawns `wrapper.sh` with `AGENTIC_OS_TRIGGER=catch-up` for any agent whose *latest* run is `status: "missed"`. Appends a one-line `[tick] ...` to `<aos_home>/tick.log`. Default stdout mirrors the log line; `--json` emits a `TickSummary` record.

### The managed crontab block

A contiguous section of the user's crontab owned end-to-end by aos:

```
# BEGIN agentic_os (managed - do not edit)
*/10 * * * * '/path/to/aos' tick >> '/path/to/aos_home/tick.log' 2>&1 # agentic_os:__tick__
0 9 * * 1,2,3,4,5 '/path/to/aos_home/wrapper.sh' '/path/to/aos_home' 'ping' 'ping' '/path/to/agents/ping.sh' # agentic_os:ping
# END agentic_os
```

The block is rebuilt deterministically from the discovered agents and the configured `tick_interval`; everything outside the markers is left untouched. The CLI binary's absolute path is baked into the `__tick__` line at write time so cron's minimal PATH doesn't need to include the install dir.

### The miss record (one per agent, at most)

A `runs/miss-<agentId>-<expectedAt>.json` file with `status: "missed"`, `startedAt = expected slot`, and `endedAt = null`. Only the **latest** uncovered slot per agent is persisted — when a newer slot is detected, the older miss is deleted and replaced. Multi-slot outages deliberately collapse to one entry per agent so the dashboard surfaces one row per behind agent.

### The catch-up gate

A catch-up is fired only when an agent's *latest* run (by `startedAt`) has `status: "missed"`. Any other status — `running`, `success`, `error`, or even a prior catch-up that succeeded or failed — supersedes the miss and prevents auto-fire. **Catch-ups don't auto-retry on failure**: a failed catch-up writes `status: "error"`, and the next tick sees that as the latest run and stops.

## User Flows

### Routine cron-driven tick

1. Cron fires `aos tick` every `tick_interval`.
2. The scanner walks `agents/`; for each scheduled agent, `DetectMissed` finds the latest cron slot ≤ now that no real run covers.
3. New misses are persisted (replacing stale ones); the post-write runs slice is reused.
4. If `catchup_enabled` is on, every agent whose latest run is missed gets a catch-up wrapper spawned.
5. One `[tick] ...` line is appended to `tick.log`. The `crontab` field of the summary reports `managed | empty | conflict | drift | error(...)`.

### Adding or editing an agent

1. User drops a script (or edits a sidecar) under `<aos_home>/agents/`.
2. User runs `aos refresh` (or `aos schedule` does it in-process).
3. The managed block is rebuilt and only rewritten if it differs from what's already there — running refresh twice is a no-op.
4. Any newly-detected misses are recorded; `runs_hard_cap` is enforced; `tick.log` is trimmed if oversized.

### Cron daemon outage

1. The cron daemon is down for two hours; three hourly slots pass un-fired.
2. Daemon comes back; the next `aos tick` runs.
3. `DetectMissed` finds the latest uncovered slot — *just the latest*, not all three.
4. The miss record for that agent is created (or, if one already existed for an earlier slot, replaced).
5. If catch-up is enabled, one wrapper is spawned with `AGENTIC_OS_TRIGGER=catch-up` and the missed slot's RFC3339 stamp as `scheduleId`.
6. That wrapper writes its `running` record; the next tick sees the latest run isn't missed and stops.

## Design Decisions

- **Refresh and tick share scan code but not write semantics.** Refresh owns the cron block; tick never touches it directly — it only *reports* the block's state (`managed | empty | conflict | drift`). Drift means the on-disk block doesn't match what'd be rebuilt now; the user runs refresh to fix it.
- **One miss per agent at a time.** Recording every uncovered slot would balloon `runs/` during outages and break the "agents currently behind" UI into a noisy multi-row mess. Collapsing to "the latest uncovered slot" is the deliberate granularity trade-off; once any real run covers a slot ≥ the missed one, the miss record stops re-emitting.
- **Catch-ups don't auto-retry.** The gate is strict: `latest == missed`. A failed catch-up leaves `latest == error`, which doesn't trigger the gate again. Auto-retry on failure would mask broken scripts behind silent loops; manual intervention is the explicit choice.
- **Health is reported alongside the work.** Refresh emits `wrapper`, `python3`, `daemon`, etc. so a degraded install (wrapper missing, cron daemon down, `python3` absent for the wrapper's atomic-write helper) is visible in one summary instead of forcing the operator to grep logs.
- **Reconciliation is idempotent and lock-protected.** Refresh acquires a file lock under `<aos_home>/.crontab.lock` before touching crontab; a contended lock skips this round rather than racing. The block is only written when its content actually changed — `cron=unchanged` is a healthy result.
- **The cron block is structured, not free-form.** Markers bracket exactly one block; duplicate markers are detected and reported as `conflict`, after which `--force` (not yet exposed) would be needed to purge and rebuild. Crontab content outside the markers is preserved verbatim across refreshes.
- **`tick.log` keeps its historical `[tick] ...` shape regardless of `--json`.** Tail consumers (the dashboard log viewer, ad-hoc grep) depend on the prefix; the log line shape is independent of stdout's structured form.
- **`tick_interval` is parsed strictly.** Whole minutes 1..59 → `*/N`, whole hours 1..23 → `0 */H`. Sub-minute precision, non-divisible hour intervals, and 24h+ all error out — cron's `*/N` step syntax can't express them cleanly, and silently rounding would surprise the operator.
