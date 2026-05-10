# Scheduler Engine — Design

## Overview

The scheduler is a **viewer and editor on top of the system's cron**. The Electron app does not run a ticker of its own; it generates a managed section in the user's crontab, and a small wrapper script invoked by cron records each run to disk. When the app launches (and every five minutes while open) it cross-references expected ticks against actual runs to detect what didn't fire.

This shape gets us three properties cleanly: (1) schedules fire even when the app is closed, (2) missed runs are visible, (3) "agent" stays a thin concept — any executable shell script the owner drops into `<userData>/data/agents/`.

## Components

### Agent

Two halves that always live together on disk: a script + an optional sidecar.

- **Script:** any executable shell file at `<userData>/data/agents/<id>.<ext>` (top-level → section "Agents") or `<userData>/data/agents/<Section>/<id>.<ext>` (first-level subdirectory name = section). Filename without extension is the agent id; ids must be unique across the whole tree.
- **Sidecar:** an optional `<id>.meta.json` next to the script, holding any combination of `schedule`, `scheduledAt`, `title`, `description`. None of the fields are required — the dashboard writes the sidecar when the user gives the agent a schedule or edits its description. `scheduledAt` (ISO timestamp) is automatically set whenever the schedule changes; missed-run detection ignores expected ticks before this point so newly-set schedules don't backfill historical misses. Section is **never** in the sidecar: it always comes from the script's parent folder.

The two halves are folded into a unified `Agent` view by the scanner. An agent with a script but no sidecar shows on the dashboard with default title / description / no schedule. A sidecar with no matching script is silently ignored — copying or deleting an agent is `cp <id>.*` / `rm <id>.*`.

### Schedule spec

Schedules attached to agents use the same structured shape as before:

- **hourly** — every N hours (1–12) at minute M (0–59).
- **daily** — a set of weekdays at HH:MM.

Compiled to a cron expression internally; the owner never sees the cron string.

### Wrapper

A bash script (`<userData>/data/wrapper.sh`, copied from `resources/wrapper.sh` on every app start). Each crontab line invokes the wrapper with the data dir, schedule id, agent id, and script path. The wrapper:

1. Writes a `running` meta file as the first action so the UI can show in-flight runs.
2. Runs the agent script; captures combined stdout/stderr to `<run-id>.out`.
3. Rewrites the meta file with `endedAt`, `status`, `exitCode`.

Meta + output are two files per run, both inside `<userData>/data/runs/`. No JSONL appends, no escaping inside bash — the only structured field that needs JSON encoding is the meta record, written via a small inline `python3` block.

### Run

A `JobRun` is the meta JSON written by the wrapper. The shape carries `jobId` (the agent id) plus `exitCode: number | null` and `outputPath: string | null` (relative to the runs directory). `output` is a *summary tail* (last ~4KB) populated lazily by the main process when ingesting the meta file — full output is fetched on demand via `scheduler:read-run-output`.

### Engine

In-process glue that owns no scheduling of its own. Responsibilities:

- Install/refresh the wrapper on every start.
- Scan the agents directory tree (top level + first-level subdirs) for scripts, folding each script's `<id>.meta.json` sidecar into a unified `AgentEntry`.
- Reconcile the managed crontab section whenever schedules change.
- Watch the runs directory and broadcast changes to renderer windows.
- Sweep for missed runs every five minutes.
- Spawn the wrapper directly for manual "run now" actions, with `AGENTIC_OS_TRIGGER=manual` in the env.

## User Flows

### Schedule an agent

Owner clicks an agent card, picks `hourly` / `daily`, saves. The renderer calls `agents:set-schedule(id, spec)`; the engine writes the script's sidecar (`<id>.meta.json`), then rewrites the managed crontab block. From then on, `cron` runs the agent — independent of whether the app is open.

### Manual run

Owner clicks `[run]`. The engine spawns the wrapper as a detached process with the same arguments cron would have used, only with `AGENTIC_OS_TRIGGER=manual` and an empty `<schedule-id>`. The wrapper writes its meta file like any other run; the directory watcher picks it up and the UI refreshes. The stub `JobRun.id` returned by the IPC call is pre-generated and threaded into the wrapper as a 5th argv so the in-flight stub matches the on-disk record.

### Missed runs

Every five minutes (and once at startup) the engine enumerates the expected ticks for each agent across the last 24 hours, cross-references them with actual runs (90s tolerance for cron jitter / wake lag), and produces a `MissedRun[]` set. The dashboard shows the top three with a per-row "run now" button. There is no auto-fire — cron is the source of truth and the user decides whether a missed run is still relevant.

### Reconcile a tampered crontab

If the user (or another tool) damages the BEGIN/END markers of the managed section, the engine refuses to write more cron lines and shows a banner. The "reconcile" action requires an explicit two-click confirm and then forcibly purges every BEGIN/END pair from the crontab and reinstalls a clean managed block.

## Design Decisions

- **No in-process ticker.** The whole point of this refactor was to stop tying scheduling to whether the Electron process is alive.
- **Drop catch-up auto-fire.** During downtime, cron either ran the job or it didn't. Re-firing on app launch was a guess; surfacing missed runs and letting the user decide is more honest.
- **Per-run files, not jsonl appends.** Atomic writes via temp + rename; multiline output goes into a sibling `.out` file so JSON escaping is bounded to fixed fields.
- **Agents are files on disk, not code.** Adding a new agent means dropping a script. No code reload, no app rebuild, no in-tree registry.
- **Section comes from filesystem, not config.** Folder-as-tab is intuitive (`mv` to reorganize) and avoids a third source of truth. The sidecar only stores what isn't on disk: the schedule plus optional human-readable overrides.
- **Per-script sidecars, not a central registry.** `<id>.meta.json` lives next to its script; copying or deleting an agent is `cp <id>.*` / `rm <id>.*`. There is no orphan-config failure mode and no global file the scanner has to reconcile against.
- **System dependencies are surfaced, not papered over.** If `crontab`, `python3`, or `wrapper.sh` is missing, the dashboard shows a banner explaining what to install. We don't ship a fallback path that silently degrades.
- **Conflict over silent overwrite.** When a hand-edit damages the managed crontab section, we *don't* rewrite by default. The user explicitly clicks "reconcile" (twice — confirm step) to authorize losing their edits.
