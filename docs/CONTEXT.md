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
A tagged structure (`kind: hourly | daily`) on a sidecar that describes when the agent should fire. Hourly carries `everyHours` + `minute`; daily carries `days` + `hour` + `minute`. Compiled to a crontab(5) expression by `CompileToCron`.
_Avoid_: cadence, trigger spec, recurrence.

**Schedule kind**:
The discriminator on a `Schedule` — either `"hourly"` or `"daily"`. The CLI **infers** the kind from which flags the user passes to `aos schedule`; there is no `--kind` flag.
_Avoid_: schedule type.

**Run**:
One execution attempt for one agent, persisted as `<aos_home>/runs/<run-id>.json` plus an optional sibling `<run-id>.out` holding captured stdout/stderr. The `status` field is one of `running | success | error | missed`. The Go type is `scheduler.Run`.
_Avoid_: execution, invocation, log entry, job run (historic name).

**Run ID**:
The stable identifier for one run; either the wrapper's `<unix-ms>-<rand4>` format, the engine's `<unix-ms>-<rand4>` produced by `NewRunID`, or — for misses — `miss-<agentId>-<expectedAt>` (colons replaced with `-`).
_Avoid_: job id.

**Trigger**:
Why a run was fired. One of `schedule` (cron), `manual` (`aos run`), or `catch-up` (a tick-driven retry for a previously-missed slot). Conveyed to `wrapper.sh` via `AGENTIC_OS_TRIGGER`.
_Avoid_: cause, source.

**Missed run** (aka **miss record**):
A `runs/miss-*.json` record with `status: "missed"` describing the latest scheduled slot the wrapper never fired for an agent. At most **one** miss record per agent exists on disk at a time — a newer uncovered slot deletes and replaces the older record. Multi-slot outages deliberately collapse to one entry.
_Avoid_: missed slot (use this only when speaking of the abstract time point, not the persisted record).

**Catch-up**:
A run with `trigger: "catch-up"` that `aos tick` spawns when an agent's *latest* run is `status: "missed"`. The trigger condition is strict: any other status (`running`, `success`, `error`) on the latest run skips the catch-up, so a failed catch-up does **not** auto-retry.
_Avoid_: retry, makeup run.

**Tick**:
One execution of `aos tick`, normally invoked by cron via the managed `__tick__` entry at `tick_interval` (default `10m`). One tick = detect misses, fire catch-ups (if enabled), log a summary.
_Avoid_: poll, heartbeat.

**Refresh**:
One execution of `aos refresh`, which rescans `agents/`, records newly-detected misses, reconciles the managed crontab block, sweeps the runs cap, and trims `tick.log`. Idempotent — running twice in a row is a no-op.
_Avoid_: reload, resync.

**Managed crontab block**:
The contiguous section of the user's crontab bracketed by `# BEGIN agentic_os (managed - do not edit)` and `# END agentic_os`. Owned end-to-end by `aos refresh` / `aos uninstall`. Contains one line per scheduled agent plus a `__tick__` entry.
_Avoid_: cron section, our cron lines.

**Wrapper**:
The `wrapper.sh` script embedded into the binary and written to `<aos_home>/wrapper.sh` by `aos init`. Cron and `aos run` both invoke it; it owns the start/end/exit/output capture contract that produces the on-disk `<run-id>.json` + `<run-id>.out`.
_Avoid_: shim, runner.

**Stub** (aka **Run stub**):
The in-memory record `aos run` (or the catch-up spawn path) prints *before* the wrapper has finished. Same shape as a real `Run` with `status: "running"`, `endedAt: null`, `output: ""`. The persisted file the wrapper later writes uses the **same `id`** as the stub so callers can correlate.
_Avoid_: placeholder, pending record.

**Estimate**:
The duration `aos run` predicts a run will take, computed as the average elapsed time of up to the 10 newest completed runs for the agent. `-1` in JSON (`"none"` in human output) when there is no completed history yet.
_Avoid_: ETA, prediction.

## Relationships

- An **Agent** is identified by its `id` (filename stem) and may have at most one **Sidecar**.
- A **Sidecar** may carry at most one **Schedule**; clearing it (via `aos schedule --off`) removes the sidecar entirely if no other fields remain.
- An **Agent** with a **Schedule** contributes one line to the **Managed crontab block**; agents with `Warnings` (e.g. `not-executable`) are skipped from cron but still appear in listings.
- A **Run** belongs to exactly one **Agent** (`agentId` field) and is identified by its **Run ID**.
- A **Missed run** is created by **Refresh** or **Tick** when no real run covers an agent's most-recent past slot; it is **replaced** (not appended) when a newer slot becomes uncovered.
- A **Catch-up** is spawned by **Tick** only when an agent's latest **Run** is a **Missed run**, and produces a normal Run whose `scheduleId` is the missed slot's RFC3339 stamp.
- The **Stub** and the wrapper-written **Run** share a single **Run ID**; the wrapper's atomic write replaces the stub.

## Example dialogue

> **Operator:** "Why didn't my hourly agent fire at 14:00 — I see a `miss-ping-2026-05-17T14-00-00Z.json` in `runs/`?"
> **Maintainer:** "The wrapper didn't run that slot — either cron was down, the laptop was asleep, or the wrapper couldn't exec. `aos tick` recorded the **Missed run**; if `catchup_enabled` is on, the next tick will spawn a **Catch-up** for it."
>
> **Operator:** "But I see only one miss record, even though we were offline for six hours."
> **Maintainer:** "Yes — only one **Missed run** per **Agent** exists at a time. Multi-slot outages collapse to one entry so the dashboard shows one row per behind agent."

## Flagged ambiguities

- **`schedule` field on the sidecar vs the column in `aos list`.** On disk it is the structured **Schedule**; in the human table it is a one-line summary (`weekdays 23:00`, `hourly :05`, etc.). The JSON output of every verb always carries the structured form.
- **`description` (long form) vs the listing column.** `aos describe` writes/reads the full text; `aos list` truncates the first line of it to fit the column. The JSON contract carries the full string in both places.
