# scheduler — Design

## Overview

Two verbs share one body of logic. `aos refresh` is the *idempotent reconciler*: rescan agents, hand the result to the platform backend (`backend.Sync`) so it writes/updates/removes the per-agent plist files (macOS) or `.timer` + `.service` pairs (Linux), record newly-detected missed slots, sweep the runs cap, trim `tick.log`. `aos tick` is the *periodic heartbeat*: the platform backend invokes it every `tick_interval` (default `1h`) via a managed `__tick__` LaunchAgent / `agentic-os-tick.timer`; one tick detects misses, sweeps stale `running` records, probes the backend's drift state, and appends one line to `tick.log`.

Crucially, **aos does not dispatch missed slots itself**. Both launchd and systemd-user timers carry native make-up-on-wake semantics (launchd fires the next `StartCalendarInterval` slot after wake; systemd's `Persistent=true` on `.timer` units replays a missed activation on the next service-manager start). The previous cron-era catch-up loop was redundant against this and is gone.

## Components

### `aos refresh`

The full reconciliation pass. Rescans `<aos_home>/agents/`, builds a `backend.Spec` (one `AgentJob` per scheduled, non-warned agent plus a `TickJob` for the periodic tick), and calls `backend.Sync`. The backend renders each job into its platform format, atomic-writes the file if its contents drift from what's expected, and bootstraps (`launchctl bootstrap gui/$UID`) or `enable --now`s the unit. Orphans — files in the namespace that don't correspond to a live agent — are booted-out and unlinked. Refresh also records newly-detected miss records, trims `tick.log` if it exceeds the byte cap, and sweeps `runs/` down to `runs_hard_cap`. Reports per-surface health: `backend`, `wrapper`, `python3`, `backendHealth`, optionally `lingerState` on Linux, plus `log` and `runs`.

### `aos tick`

One scheduler cycle, fired by the platform backend's `__tick__` job. Detects the latest uncovered slot per scheduled agent and persists it as a `miss-*.json` (replacing any older miss record for that agent); runs the stale-running sweep over the post-write runs slice; probes `backend.State` to surface drift; appends a one-line `[tick] ...` to `<aos_home>/tick.log`. Default stdout mirrors the log line; `--json` emits a `TickOutcome` record.

### The managed namespace

The per-platform set of files aos owns end-to-end.

- **macOS** — every `.plist` under `~/Library/LaunchAgents/` whose label starts with `com.agenticos.`. Each agent owns one file (`com.agenticos.<agent-id>.plist`); the tick is `com.agenticos.__tick__.plist`. Bootstrapped into the per-user GUI domain (`gui/$UID`) so jobs inherit the login Keychain — the whole reason aos moved off cron on macOS.
- **Linux** — every `.timer` + `.service` pair under `~/.config/systemd/user/` whose stem starts with `agentic-os-`. Each agent owns one pair (`agentic-os-<agent-id>.{timer,service}`); the tick is `agentic-os-tick.{timer,service}`. Enabled with `systemctl --user enable --now`.

In both cases, each agent owns its own file(s) (there is no shared block to corrupt). Files outside the namespace are never touched. Each agent's file is rendered deterministically from its `ScheduleSpec`; the wrapper path, agent id, and script path are baked in at write time. The CLI binary's absolute path is baked into the tick entry so the resolved binary is invoked regardless of the caller's `PATH`.

### The miss record (one per agent, at most)

A `runs/miss-<agentId>-<expectedAt>.json` file with `status: "missed"`, `startedAt = expected slot`, and `endedAt = null`. Only the **latest** uncovered slot per agent is persisted — when a newer slot is detected, the older miss is deleted and replaced. Multi-slot outages deliberately collapse to one entry per agent so the dashboard surfaces one row per behind agent. The record is for visibility only; the platform backend's native makeup will fire the next slot on its own.

### Drift state

The backend reports its current state via `Backend.State(spec)`, which returns `managed | drift | empty`:

- `managed` — every file aos expects is present and its on-disk contents match a deterministic re-render of the spec.
- `drift` — at least one file is missing, extra, or differs from the expected render.
- `empty` — both the expected set and the on-disk set are empty.

There is no `conflict` state. Each agent owns its own file, so there is no shared resource a user could corrupt; a hand-edited plist or unit is treated as drift and silently overwritten by the next refresh.

### The stale-running sweep

Lives in `internal/scheduler/stale.go`. Walks the in-memory runs slice produced by `RecordMissedRuns` (or freshly loaded if `tick` ran into an issue) and rewrites every `Run` whose `status == "running"` and `startedAt` is older than `StaleRunningThreshold` (1 h) to `status="error"`, `error="no completion record"`, `exitCode=1`. Covers wrappers killed mid-run by a backend reload (`bootout`, `disable --now`), an OOM, or a power loss between the wrapper's "running" write and its terminal write. The tick summary reports the rewrite count as `staleResolved`.

## User Flows

### Routine backend-driven tick

1. The platform backend fires `aos tick` every `tick_interval`.
2. The scanner walks `agents/`; for each scheduled agent, `DetectMissed` finds the latest scheduled slot ≤ now that no real run covers (via `ScheduleSpec.NextSlot` iteration — no third-party cron parser dependency).
3. New misses are persisted (replacing stale ones); the post-write runs slice is reused for the next step.
4. `SweepStaleRunning` rewrites stale `running` records.
5. `backend.State(spec)` is probed for the drift summary string.
6. One `[tick] ...` line is appended to `tick.log`. The summary's `backend` field reports `managed | drift | empty | error(<reason>)`.

### Adding or editing an agent

1. User drops a script (or edits a sidecar) under `<aos_home>/agents/`.
2. User runs `aos refresh` (or `aos schedule` does it in-process).
3. `backend.Sync` is called; only the files whose rendered contents differ from on-disk content are atomic-written, then `bootout`+`bootstrap`-cycled (macOS) or re-`enable --now`-d (Linux). Removed agents are booted-out/disabled and unlinked. Running refresh twice in a row is a no-op (`wrote=0 unchanged=N removed=0`).
4. Any newly-detected misses are recorded; `runs_hard_cap` is enforced; `tick.log` is trimmed if oversized.

### Sleep / wake outage

1. The user's laptop is asleep for two hours; three hourly slots pass un-fired.
2. The laptop wakes.
3. **launchd / systemd-user fire the next slot themselves** (native makeup) — aos has nothing to do with this.
4. The next `aos tick` runs (either fired by the wake-up itself or on its normal cadence). `DetectMissed` finds the latest uncovered slot — *just the latest*, not all three — and records it as a `miss-*.json` for visibility.
5. As soon as the makeup wrapper writes its `running` record (within seconds of wake), it covers the missed slot from the detector's perspective; the next tick clears the miss.

### Wrapper killed mid-run

1. A user installs a new script. `aos refresh` rewrites every plist; on macOS, this triggers a `bootout`+`bootstrap` cycle which SIGTERMs any in-flight wrapper.
2. The wrapper's SIGTERM trap fires: it writes an `error` record (`"interrupted by reload"`, exit 143) before exit, so no orphaned `running` entry is left.
3. If the trap didn't fire (process killed -9, OOM, power loss), the next `aos tick`'s stale-running sweep rewrites the orphaned `running` record to `error` an hour after `startedAt`. The user sees the failure in the dashboard; the platform's native makeup has already restarted the schedule.

## Design Decisions

- **Refresh and tick share scan code but not write semantics.** Refresh owns `backend.Sync` (writes to the namespace); tick never touches it directly — it only *reports* the namespace's drift state. The user runs refresh to fix drift.
- **No catch-up dispatch.** Both backends fire missed slots on wake natively. Re-firing from aos's tick loop would have produced double-runs in many real-world cases. The miss record is now just for visibility; the actual makeup is the OS's responsibility.
- **One miss per agent at a time.** Recording every uncovered slot would balloon `runs/` during outages and break the "agents currently behind" UI into a noisy multi-row mess. Collapsing to "the latest uncovered slot" is the deliberate granularity trade-off; once any real run covers a slot ≥ the missed one, the miss record stops re-emitting.
- **Stale `running` records get rewritten, not deleted.** A wrapper killed mid-run leaves a `running` record that would otherwise be readable forever in the dashboard. The sweep rewrites it to `error` with an explicit message so the operator sees that it *did* fail, just without a normal terminal write. Deleting would erase evidence; rewriting preserves it.
- **Drift is silently overwritten.** A user who hand-edits a plist or a `.timer` will see the refresh write the canonical version on top. There is no `conflict` state to refuse-and-skip on, because there is no shared resource to corrupt — every agent owns its own file. If the user *wanted* their hand-edit to win, the right answer is to extend the sidecar; the manage-by-aos contract is end-to-end.
- **Health is reported alongside the work.** Refresh emits `wrapper`, `python3`, `backendHealth`, and (Linux) `lingerState` so a degraded install (wrapper missing, no python3, linger off on a headless host) is visible in one summary instead of forcing the operator to grep logs.
- **Per-agent files, not a shared block.** This is the architectural change that lets drift be silently overwritten and lets orphan cleanup be unconditional: aos always knows which files are "ours" by namespace prefix; everything outside that prefix is by definition not aos's concern.
- **`tick.log` keeps its historical `[tick] ...` shape regardless of `--json`.** Tail consumers (the dashboard log viewer, ad-hoc grep) depend on the prefix; the log line shape is independent of stdout's structured form.
- **`tick_interval` defaults to 1 h.** The cron era ran every 10 min because catch-ups had to be polled for. With native makeup taking over the make-up role, the only work the tick does is record misses, sweep stale running, and probe drift — all cheap, none time-sensitive. Hourly is fine.
- **Linux requires linger on headless hosts.** Without `loginctl enable-linger $USER`, user timers stop when the operator's session ends. `aos init` probes for it and prompts with `sudo` to enable; `aos refresh` re-warns through `LingerState`. On a desktop with an always-on GUI session, the warning is silenced.
