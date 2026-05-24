# aos CLI — Context

The shared vocabulary the `aos` CLI uses for its domain. Most concepts surface in multiple verbs, the on-disk layout, the JSON contract, and the TUI — so they live here rather than in per-feature glossaries. Feature-local terms (if any) live in `docs/features/{feature}/CONTEXT.md`.

## Language

**aos_home**:
The user-chosen directory that holds `agents/`, `runs/`, `wrapper.sh`, and `tick.log`; its absolute path is stored in `~/.config/aos/config.toml` under `aos_home`.
_Avoid_: data dir, install dir, root.

**Agent**:
One user-provided script discoverable under `<aos_home>/agents/`, identified by a stable `id` (filename minus extension), optionally paired with a `.meta.json` sidecar.
_Avoid_: job, task, script (the on-disk file is *a script*, but the abstraction `aos` operates on is *an agent*).

**Sidecar**:
The optional JSON file at `<script-path>.meta.json` that carries the agent's `schedule`, `scheduledAt`, `title`, and `description`. Written by `aos schedule`, `aos describe`, and the details popup; consumed by every scanner.
_Avoid_: meta file, manifest, config.

**Section**:
The bucket an agent belongs to in listings and in the TUI. Recovered from the script's parent directory at scan time: top-level scripts land in `"Agents"`; a first-level subdirectory's name becomes the section.
_Avoid_: group, category, folder.

**Schedule**:
A tagged structure (`kind: hourly | daily`) on a sidecar that describes when the agent should fire. Hourly carries `everyHours` + `minute`; daily carries `days` + `hour` + `minute`. The Go type is `schedspec.ScheduleSpec`; the backend rendering layer maps it to a launchd `StartCalendarInterval` or a systemd `OnCalendar` string.
_Avoid_: cadence, trigger spec, recurrence.

**Schedule kind**:
The discriminator on a `Schedule` — either `"hourly"` or `"daily"`. The CLI **infers** the kind from which flags the user passes to `aos schedule`; there is no `--kind` flag.
_Avoid_: schedule type.

**Backend**:
The platform-native scheduler aos drives. Defined as the `Backend` interface in `internal/scheduler/backend/backend.go` (`Sync`, `Remove`, `State`); implemented by `LaunchdBackend` on macOS and `SystemdBackend` on Linux. `backend.Select` picks one at runtime by GOOS — unsupported platforms hard-error.
_Avoid_: scheduler driver, OS scheduler (used loosely below in flavor text, but the named concept is "Backend").

**LaunchAgent**:
The macOS unit type aos writes. A `.plist` file at `~/Library/LaunchAgents/com.agenticos.<id>.plist`, loaded into the per-user GUI domain (`gui/$UID`) via `launchctl bootstrap`. Runs inside the login session, which is the whole reason aos uses launchd instead of cron — only in-session jobs can reach the login Keychain.
_Avoid_: LaunchDaemon (those are root-domain, not what aos uses), plist (the file is *a plist*; the running thing is *a LaunchAgent*).

**User timer**:
The Linux unit pair aos writes — a `.timer` and a `.service` under `~/.config/systemd/user/agentic-os-<id>.{timer,service}`, enabled with `systemctl --user enable --now`. The timer carries `Persistent=true` for native make-up-on-wake.
_Avoid_: cron job, systemd unit (ambiguous between system-wide and user units).

**Managed namespace**:
The per-platform set of files aos owns end-to-end. On macOS, every plist matching `com.agenticos.*` under `~/Library/LaunchAgents/`. On Linux, every `.service` / `.timer` matching `agentic-os-*` under `~/.config/systemd/user/`. Refresh writes/updates/removes entries within the namespace; anything else on the filesystem is untouched.
_Avoid_: our jobs, scheduler block.

**Native makeup**:
The platform-provided behavior that fires a missed schedule shortly after wake. launchd does this implicitly for `StartCalendarInterval` jobs; systemd does it when the `.timer` has `Persistent=true`. This is what replaced the old aos-managed catch-up dispatch — the OS handles missed-while-asleep slots, and aos only records the per-agent "latest uncovered slot" through `Missed run` for visibility.
_Avoid_: catch-up, retry (those are concepts that no longer exist in the system).

**Linger** _(Linux-only)_:
The systemd-logind setting that keeps a user's manager running when no session is open. Without it, user timers stop firing when the operator logs out — fine on a desktop with a permanent GUI session, fatal on a headless host. `aos init` probes `loginctl show-user --property=Linger` and prompts for `sudo loginctl enable-linger $USER` when off on a headless box; `aos refresh` re-surfaces the state via `RefreshOutcome.LingerState`.
_Avoid_: persistent user mode, auto-login.

**Run**:
One execution attempt for one agent, persisted as `<aos_home>/runs/<run-id>.json` plus an optional sibling `<run-id>.out` holding captured stdout/stderr. The `status` field is one of `running | success | error | missed`. The Go type is `scheduler.Run`.
_Avoid_: execution, invocation, log entry, job run (historic name).

**Run ID**:
The stable identifier for one run; either the wrapper's `<unix-ms>-<rand4>` format, the engine's `<unix-ms>-<rand4>` produced by `NewRunID`, or — for misses — `miss-<agentId>-<expectedAt>` (colons replaced with `-`).
_Avoid_: job id.

**Trigger**:
Why a run was fired. One of `schedule` (the platform backend fired the wrapper) or `manual` (`aos run`). Conveyed to `wrapper.sh` via `AGENTIC_OS_TRIGGER`.
_Avoid_: cause, source.

**Missed run** (aka **miss record**):
A `runs/miss-*.json` record with `status: "missed"` describing the latest scheduled slot the wrapper never fired for an agent. At most **one** miss record per agent exists on disk at a time — a newer uncovered slot deletes and replaces the older record. Multi-slot outages deliberately collapse to one entry. Detected by `aos tick` / `aos refresh`; aos does **not** spawn a follow-up run for it (native makeup is the platform's responsibility — see `Native makeup`).
_Avoid_: missed slot (use this only when speaking of the abstract time point, not the persisted record).

**Stale-running sweep**:
The pass `aos tick` makes that rewrites any `Run` with `status="running"` and `startedAt` older than 1 h to `status="error"` with `error="no completion record"`. Covers the case where the wrapper was killed by an OS reload, an OOM, or a `bootout`/`disable` between `running` write and terminal write. Lives in `internal/scheduler/stale.go`; the tick summary surfaces the count as `staleResolved`.
_Avoid_: zombie sweep, orphan cleanup.

**Tick**:
One execution of `aos tick`, normally invoked by the platform backend's tick LaunchAgent / `agentic-os-tick.timer` at `tick_interval` (default `1h`). One tick = scan agents, record missed slots, sweep stale-running records, probe backend drift state, append a summary to `tick.log`.
_Avoid_: poll, heartbeat.

**Refresh**:
One execution of `aos refresh`, which rescans `agents/`, records newly-detected misses, calls `backend.Sync` to reconcile the managed namespace (write/update/remove plists or unit files, bootstrap/boot-out as needed), sweeps the runs cap, and trims `tick.log`. Idempotent — running twice in a row writes nothing new.
_Avoid_: reload, resync.

**Wrapper**:
The `wrapper.sh` script embedded into the binary and written to `<aos_home>/wrapper.sh` by `aos init`. The platform backend and `aos run` both invoke it; it owns the start/end/exit/output capture contract that produces the on-disk `<run-id>.json` + `<run-id>.out`. Traps SIGTERM/SIGINT so a backend reload writes an `error` record with `"interrupted by reload"` instead of orphaning a `running` entry.
_Avoid_: shim, runner.

**Stub** (aka **Run stub**):
The in-memory record `aos run` prints *before* the wrapper has finished. Same shape as a real `Run` with `status: "running"`, `endedAt: null`, `output: ""`. The persisted file the wrapper later writes uses the **same `id`** as the stub so callers can correlate.
_Avoid_: placeholder, pending record.

**Estimate**:
The duration `aos run` predicts a run will take, computed as the average elapsed time of up to the 10 newest completed runs for the agent. `-1` in JSON (`"none"` in human output) when there is no completed history yet.
_Avoid_: ETA, prediction.

## Relationships

- An **Agent** is identified by its `id` (filename stem) and may have at most one **Sidecar**.
- A **Sidecar** may carry at most one **Schedule**; clearing it (via `aos schedule --off`) removes the sidecar entirely if no other fields remain.
- An **Agent** with a **Schedule** contributes one entry to the **Managed namespace** (one plist on macOS; one .timer + .service pair on Linux). Agents with `Warnings` (e.g. `not-executable`) are skipped from the namespace but still appear in listings.
- A **Run** belongs to exactly one **Agent** (`agentId` field) and is identified by its **Run ID**.
- A **Missed run** is created by **Refresh** or **Tick** when no real run covers an agent's most-recent past slot; it is **replaced** (not appended) when a newer slot becomes uncovered. **Native makeup** by the platform backend is what re-fires the slot — aos does not spawn it.
- The **Stale-running sweep** runs every **Tick**; the resulting count surfaces as `staleResolved` in the tick summary.
- The **Stub** and the wrapper-written **Run** share a single **Run ID**; the wrapper's atomic write replaces the stub.

## Example dialogue

> **Operator:** "Why didn't my hourly agent fire at 14:00 — I see a `miss-ping-2026-05-17T14-00-00Z.json` in `runs/`?"
> **Maintainer:** "The wrapper didn't run that slot — most likely the laptop was asleep or logged out. `aos tick` recorded the **Missed run**. The platform backend will fire the next slot on its own (launchd does it implicitly; systemd does it because the `.timer` has `Persistent=true`) — that's the **Native makeup** behavior. No catch-up is dispatched by aos."
>
> **Operator:** "But I see only one miss record, even though we were offline for six hours."
> **Maintainer:** "Yes — only one **Missed run** per **Agent** exists at a time. Multi-slot outages collapse to one entry so the dashboard shows one row per behind agent."

## Flagged ambiguities

- **`schedule` field on the sidecar vs the column in `aos list`.** On disk it is the structured **Schedule**; in the human table it is a one-line summary (`weekdays 23:00`, `hourly :05`, etc.). The JSON output of every verb always carries the structured form.
- **`description` (long form) vs the listing column.** `aos describe` writes/reads the full text; `aos list` truncates the first line of it to fit the column. The JSON contract carries the full string in both places.
- **`Backend` (interface) vs `backend` (JSON field).** The Go type is `backend.Backend`; the `refresh --json` field is `backend` (a `BackendSyncOutcome` with `state`, `wrote`, `removed`, …); the `tick --json` field is also `backend` (a plain string carrying the drift state).
