# agent-management — Design

## Overview

Agent management is the "read the filesystem, write the sidecar" surface. An agent is just a script the user drops into `<aos_home>/agents/`; the CLI scans the directory, surfaces what it found, and lets the operator edit a small JSON sidecar next to each script. None of these verbs run anything — they only read metadata and write sidecars. Platform-backend reconciliation lives in the scheduler feature; `aos schedule` triggers a refresh after a successful write so the launchd / systemd-user entries stay in sync, but the backend logic itself is not implemented here.

## Components

### `aos list`

Enumerates every agent visible under `<aos_home>/agents/`. Top-level scripts fall under section `"Agents"`; one level of subdirectories produces section buckets named after the directory. Output is a styled lipgloss table (one row per agent: id, section, schedule summary, warnings, description) or — under `--json` — `{ agents: [...], issues: [...] }` with the structured schedule shape.

### `aos describe <id> [text]`

Shows the **full record** for one agent — same per-agent shape as a `list` item — and optionally rewrites the description in a single call. The optional second positional argument is the new description (empty string clears it). Doesn't trigger a refresh because descriptions don't affect the backend's plists / unit files.

### `aos schedule <id> ...`

Sets or clears an agent's schedule. The kind is **inferred from the flags** the operator passes (`--every-hours` ⇒ hourly, `--hour`/`--days` ⇒ daily, `--off` ⇒ clear). Conflicting flag combinations are rejected outright rather than silently picking a winner. After a successful sidecar write, an in-process refresh runs so the platform-backend entry for the agent (the plist or `.timer`+`.service` pair) reflects the change immediately.

## User Flows

### Discovering agents

1. User runs `aos list`.
2. The scanner walks `agents/` and one level of subdirectories, finding files with supported extensions (`.sh`, `.bash`, `.zsh`, or no extension with a `#!` shebang).
3. Each found agent is paired with its optional `<id>.meta.json` sidecar.
4. Agents missing `+x` get a `not-executable` warning but still appear in the list.
5. Duplicate ids (same filename stem in two locations) are dropped first-wins and surfaced as `issues` on stderr / in the JSON payload.

### Inspecting one agent

1. User runs `aos describe <id>` to see the full record.
2. If the agent has a schedule, the structured spec is printed (kind, days/hour/minute or everyHours/minute) plus the `scheduledAt` stamp.
3. To set a description: `aos describe <id> "Pings the API every weekday at 9am"`. To clear: `aos describe <id> ""`.

### Setting a schedule

1. User runs `aos schedule my-agent --every-hours 3 --minute 0` (or `--hour 9 --minute 0 --days mon-fri`, etc.).
2. Flags are parsed and the kind is inferred. Conflicts (e.g. `--every-hours` plus `--days`) error out before any disk write.
3. The sidecar is updated atomically (temp + rename). `scheduledAt` is bumped to "now" **only if** the spec actually changed.
4. An in-process refresh runs and reconciles the platform backend (writes / updates the agent's plist or unit pair, bootstraps / `enable --now`s as needed). If the refresh fails, the schedule write still succeeded and the failure is reported under `refresh.error` (JSON) or as a `warn:` line on stderr (human).
5. `aos schedule my-agent --off` clears the schedule and removes the agent's backend entry in the same call.

## Design Decisions

- **Section is recovered, never stored.** A script's section is derived from its parent directory at scan time, so moving a script between subdirectories silently changes its section without any sidecar edit. This keeps the sidecar shape minimal and matches how a user thinks ("this script is in `assistant/`, so it's in the assistant section").
- **The CLI shape mirrors the sidecar shape.** No `--kind` flag exists — passing `--every-hours` *means* hourly, passing `--hour`+`--days` *means* daily. The on-disk `schedule.kind` field matches what the flags imply, and the JSON output of every verb keeps the structured form.
- **First-wins for duplicates.** When two scripts share an id, the scanner keeps the first one it encountered (sorted order) and surfaces the conflict as an issue. The runner-side behavior is deterministic — the platform backend always fires the agent `list` and `describe` agree on.
- **`scheduledAt` is "when this schedule started," not "when this sidecar was last touched."** Days are compared as sets, so reordering them doesn't bump it; editing a description doesn't either. This lets downstream tooling reason about how long a schedule has been in effect.
- **Empty meta is no meta.** A sidecar that would otherwise become `{}` is removed from disk. The scanner treats "no sidecar" and "empty sidecar" identically, so this keeps the on-disk state minimal.
- **Schedule writes refresh the backend in-process.** A user setting a schedule has the obvious expectation that the next run will actually fire — so the verb runs an embedded `runRefresh()` (which drives `backend.Sync`) instead of asking the user to also run `aos refresh`.
