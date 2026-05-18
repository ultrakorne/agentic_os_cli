# agent-management — Technical

## Architecture

Three verbs in `cmd/aos/` (`list.go`, `describe.go`, `schedule.go`) compose against one shared scanner and one shared sidecar store in `internal/scheduler/`. The scanner produces a flat `ScanResult` (agents + issues); the sidecar store (`meta_store.go`) owns **both** reads (`ReadMeta`) and writes (`WriteSchedule`, `WriteDescription`) of `<id>.meta.json`, plus the atomic-rename + empty-file-deletion invariants. The scanner reads each agent's sidecar via `ReadMeta`, so there is one read path for the whole codebase. JSON branches go through a single `agentRecord(...)` helper in `cmd/aos/format.go` so `aos list` items and `aos describe` records share one shape. `aos schedule` calls `RunRefresh()` (from the scheduler feature) in-process after a successful write.

The TUI popup (`cmd/aos/start_details.go`) consumes the `AgentMeta` returned by `WriteSchedule`/`WriteDescription` directly when the user saves — no rescan needed. The parent dashboard mirrors the update through `agentMetaUpdatedMsg` in `start_model.go:applyMetaUpdate`.

## Source Files

| File | Role |
|------|------|
| `cmd/aos/list.go` | `aos list` verb: scan + JSON or styled table render; day-set humanization (`weekdays`, `weekends`, `everyday`) |
| `cmd/aos/describe.go` | `aos describe` verb: read + optional description write; renders the same per-agent shape as `list` |
| `cmd/aos/schedule.go` | `aos schedule` verb: flag parsing, kind inference, days parser, in-process refresh, both output modes |
| `cmd/aos/schedule_test.go` | Flag-conflict + days-parsing tests |
| `cmd/aos/format.go` | `sanitize` + `agentRecord` (the canonical per-agent JSON shape) |
| `cmd/aos/format_test.go` | `agentRecord` shape tests |
| `internal/scheduler/scanner.go` | Directory walk, extension/shebang gating, executability check, duplicate detection; delegates sidecar reads to `meta_store.ReadMeta` |
| `internal/scheduler/scanner_test.go` | Scanner rules tests (extensions, sections, duplicates, hidden files, unreadable meta) |
| `internal/scheduler/spec.go` | `Weekday`, `ScheduleSpec`, `AgentMeta` types; `CompileToCron`; `ParseMeta` (pure bytes-→-struct) |
| `internal/scheduler/meta_store.go` | Single sidecar I/O point: `ReadMeta`, `WriteSchedule`, `WriteDescription`, `SpecsEqual`; atomic write, empty-meta deletion |
| `internal/scheduler/meta_store_test.go` | Sidecar round-trip + `scheduledAt` bump-on-change semantics |
| `internal/scheduler/lookup.go` | `FindAgentByID` (used by `describe` and `schedule`); `NotFoundError` |

## Data Model

### Sidecar (`<script>.meta.json`)

```json
{
  "schedule": {
    "kind": "daily",
    "days": ["mon", "tue", "wed", "thu", "fri"],
    "hour": 9,
    "minute": 0
  },
  "scheduledAt": "2026-05-16T12:00:00.000Z",
  "title": "Ping",
  "description": "Healthcheck"
}
```

All fields are optional. `schedule.kind` is either `"hourly"` (carrying `everyHours` 1..12 and `minute` 0..59) or `"daily"` (carrying `days` subset of `sun..sat`, `hour` 0..23, `minute` 0..59). The schema is mirrored on the renderer side in `src/shared/scheduler.ts`; tests on both sides catch compilation drift but the JSON shape is the contract.

### Scanner output (`ScanResult`)

- `Agents []Agent` — one entry per discovered script, sorted by `ID`. Each `Agent` carries `ID`, `ScriptPath`, `Section`, `MetaPath`, `Meta`, and `Warnings` (currently only `"not-executable"`).
- `Issues []ScanIssue` — non-fatal problems. Kinds: `"duplicate"` (same id in two sections, first wins), `"meta-unreadable"` (sidecar exists but can't be read — permission denied or I/O error; the agent stays in `Agents` with an empty `Meta`). Surfaced on stderr / in JSON but don't fail the scan.

## Noteworthy Behavior

- **`scheduledAt` bumps only when the schedule's *meaning* changes.** `SpecsEqual` compares day lists as sets — `["mon","wed"]` and `["wed","mon"]` are equal — so reordering the array doesn't count. Title/description edits don't bump it either. This lets downstream tools reason about "how long has this schedule been in effect."
- **Empty meta gets unlinked.** `writeMetaJSON` checks `isEmptyMeta` (all four fields zero) before writing; an empty result deletes the file instead of leaving `{}` on disk. So `aos schedule x --off` *and* `aos describe x ""` together leave the agent with no sidecar at all.
- **Sidecar writes are atomic.** `writeMetaJSON` writes to `<path>.tmp` and `os.Rename`s on top of the target. A crash never leaves a half-written sidecar; a concurrent reader sees either the old or the new file, never garbage.
- **One read path for sidecars.** `meta_store.ReadMeta(path)` is the only place that turns `<id>.meta.json` bytes into an `AgentMeta`: missing file → empty meta, no error; permission/I/O error → empty meta plus an error. The scanner calls into it (it does not re-implement reads). Write helpers (`WriteSchedule`, `WriteDescription`) return the resulting `AgentMeta` so callers — including the TUI popup — can update in-memory state without re-scanning.
- **Day-set humanization is human-only.** `humanizeDays` in `list.go` collapses three canonical sets — `mon..fri` → `weekdays`, `sat,sun` → `weekends`, full week → `everyday`. The JSON payload always carries the explicit array.
- **Hidden files and `__-prefixed` ids are skipped.** Dotfiles, `readme.md`, and any sidecar files (`*.meta.json`) are filtered out. Ids starting with `__` are reserved for managed cron entries (e.g. `__tick__`) and are dropped from scan results so a user can't accidentally collide with them.
- **Shebang fallback for extensionless files.** A file without an extension is included if its first two bytes are `#!`. This lets a user drop a Python or Node script with just `agents/cleanup` and a `#!/usr/bin/env python3` first line, no `.sh` rename needed.
- **`not-executable` agents stay in the listing but skip cron.** The scanner flags them so the dashboard can nudge the user; `aos refresh` walks `Warnings` and drops the agent from the managed block so cron never tries to exec a non-executable file. The count of warned-but-scheduled agents shows up as `issues++` in the refresh summary.
- **Flag conflicts in `aos schedule` are rejected up-front.** `--every-hours` with `--hour`/`--days`, `--off` with anything else — `parseSchedFlags` returns an error before any disk write so the operator gets a clean diagnostic instead of a half-applied state.
- **Days-string parser is strict.** Accepts a single comma list (`mon,wed,fri`) or a single inclusive range (`mon-fri`). Reverse ranges (`fri-mon`) and range-plus-comma (`mon-fri,sun`) error out — there's no canonicalization that's obviously correct, so the verb refuses to guess.
- **A failed post-write refresh doesn't fail the verb.** `aos schedule` reports `refresh.error` in JSON (or a `warn:` line on stderr) but exits 0 — the sidecar write itself was the user's intent, and the next `aos refresh` will catch up the cron block.

## Dependencies

- `internal/scheduler` — scanner, sidecar store, cron compilation.
- `cmd/aos/refresh.go` (`RunRefresh`) — invoked in-process after `aos schedule` writes.
- `github.com/spf13/cobra` — CLI plumbing.
- `charm.land/lipgloss/v2` — table + key/value styling.
