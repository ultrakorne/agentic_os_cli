# Scheduler Engine — Design

## Overview

The scheduler is a **viewer and editor on top of the system's cron**. The Electron app does not run a ticker of its own; it generates a managed section in the user's crontab, and a small wrapper script invoked by cron records each run to disk. When the app launches (and every five minutes while open) it cross-references expected ticks against actual runs to detect what didn't fire.

This shape gets us three properties cleanly: (1) schedules fire even when the app is closed, (2) missed runs are visible, (3) "agent" stays a thin concept — any executable shell script the owner drops into `<userData>/agents/`.

## Components

### Schedule

Persistent record binding an agent (`jobId`) to a recurrence (`spec`). At most one schedule per agent. Spec shapes are unchanged from earlier versions:

- **hourly** — every N hours (1–12) at minute M (0–59).
- **daily** — a set of weekdays at HH:MM.

The spec is compiled to a cron expression internally; the owner never sees the cron string.

### Agent

Any executable file in `<userData>/agents/`. The filename without extension is the `jobId`; an optional sibling `<id>.meta.json` supplies a friendly title, description, and section. Two seed scripts (`ping.sh`, `disk-free.sh`) are copied in on first run; the owner adds, edits, and deletes the rest as plain files.

### Wrapper

A bash script (`<userData>/data/wrapper.sh`, copied from `resources/wrapper.sh` on every app start). Each crontab line invokes the wrapper with the data dir, schedule id, agent id, and script path. The wrapper:

1. Writes a `running` meta file as the first action so the UI can show in-flight runs.
2. Runs the agent script; captures combined stdout/stderr to `<run-id>.out`.
3. Rewrites the meta file with `endedAt`, `status`, `exitCode`.

Meta + output are two files per run, both inside `<userData>/data/runs/`. No JSONL appends, no escaping inside bash — the only structured field that needs JSON encoding is the meta record, written via a small inline `python3` block.

### Run

A `JobRun` is the meta JSON written by the wrapper. The shape is preserved across the refactor with two new fields: `exitCode` (number / null) and `outputPath` (string / null, relative to the runs directory). `output` is now a *summary tail* (last ~4KB) populated lazily by the main process when ingesting the meta file — full output is fetched on demand via `scheduler:read-run-output`.

### Engine

In-process glue that owns no scheduling of its own. Responsibilities:

- Install/refresh the wrapper on every start.
- Discover agents by scanning the agents directory.
- Reconcile the managed crontab section whenever schedules change.
- Watch the runs directory and broadcast changes to renderer windows.
- Sweep for missed runs every five minutes.
- Spawn the wrapper directly for manual "run now" actions, with `AGENTIC_OS_TRIGGER=manual` in the env.

## User Flows

### Schedule an agent

Owner clicks an agent card, picks `hourly` / `daily`, saves. The renderer sends the schedule via IPC; the engine persists, then rewrites the managed crontab block. From then on, `cron` runs the agent — independent of whether the app is open.

### Manual run

Owner clicks `[run]`. The engine spawns the wrapper as a detached process with the same arguments cron would have used, only with `AGENTIC_OS_TRIGGER=manual` and an empty `<schedule-id>`. The wrapper writes its meta file like any other run; the directory watcher picks it up and the UI refreshes.

### Missed runs

Every five minutes (and once at startup) the engine enumerates the expected ticks for each schedule across the last 24 hours, cross-references them with actual runs (90s tolerance for cron jitter / wake lag), and produces a `MissedRun[]` set. The dashboard shows the top three with a per-row "run now" button. There is no auto-fire — cron is the source of truth and the user decides whether a missed run is still relevant.

### Orphaned schedules

A schedule whose `jobId` doesn't match a discovered agent is flagged `orphaned: true`. Crontab sync skips orphans (no broken cron line gets written). The dashboard surfaces an orphan banner that points the user to the agents folder.

### Reconcile a tampered crontab

If the user (or another tool) damages the BEGIN/END markers of the managed section, the engine refuses to write more cron lines and shows a banner. The "reconcile" action does a forced rewrite — purges every BEGIN/END pair from the crontab and reinstalls a clean managed block.

## Design Decisions

- **No in-process ticker.** The whole point of this refactor was to stop tying scheduling to whether the Electron process is alive.
- **Drop catch-up auto-fire.** During downtime, cron either ran the job or it didn't (or the laptop was asleep). Re-firing on app launch was a guess; surfacing missed runs and letting the user decide is more honest.
- **Per-run files, not jsonl appends.** Atomic writes via temp + rename; multiline output goes into a sibling `.out` file so JSON escaping is bounded to fixed fields.
- **Agents are files on disk, not code.** Adding a new agent means dropping a script. No code reload, no app rebuild, no in-tree registry to keep in sync. Optional `.meta.json` siblings carry display metadata.
- **Schedule id == agent id.** Still enforced by the dashboard, still independent in the engine. Allows future "multiple schedules per agent" without engine changes.
- **Spec stays structured.** Cron strings remain an internal detail. The editor is opinionated; raw cron is an escape hatch we haven't needed.
- **System dependencies are surfaced, not papered over.** If `crontab`, `python3`, or `wrapper.sh` is missing, the dashboard shows a banner explaining what to install. We don't ship a fallback path that silently degrades — the user finds out immediately.
- **Conflict over silent overwrite.** When a hand-edit damages the managed section, we *don't* rewrite by default. The user explicitly clicks "reconcile" to authorize losing their edits.
