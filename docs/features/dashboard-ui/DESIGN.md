# Dashboard UI — Design

## Overview

The dashboard is the product. There is no other screen — no settings, no onboarding, no empty-state hand-holding. When the owner opens the app they should see every agent at a glance, know what is scheduled and when, and be one click away from running anything. The aesthetic is retro-runtime: monospace, tight leading, bracket-button affordances (`[run]`, `[save]`), tabular numerics for everything that scans vertically.

## Components

### Top bar

`agentic-os` wordmark, version, agent count, and the active theme name on the right. The theme name is a button — clicking re-runs the loader. Useful when watching is unreliable or when the owner just wants to confirm what's loaded.

### Sections

Agents group by `section` (`Daily`, `Engineering`, `Reflection`, `Dev`). Section headers are uppercase tracking-wide accent text with a count of agents and, if any, a count of scheduled. A dotted rule fills the rest of the line. Order is fixed (`SECTION_ORDER`); unknown sections fall to the end in insertion order.

### Agent row

One row per agent. Left to right:

- `[run]` button — fires manually
- Status glyph — `●` running (pulses), `✓` last run succeeded, `✗` last run failed, `◇` scheduled but not yet run, `·` idle
- Agent id (monospace) — stable identifier the owner reads
- Description — dim, hidden on narrow widths
- Schedule summary — accent-cool if scheduled, faint if not (e.g. `weekdays @ 09:00`)
- Next-run countdown — relative-from-now, tabular (`in 4h12m`, `2d ago`)
- Expand chevron — `▸` collapsed, `▾` expanded

### Schedule editor

Inline expansion below the row. A `hourly` / `daily` mode toggle, the controls for the chosen mode (steppers for numbers, day pills for weekdays), a live "next run" preview computed by the engine, and `[save]` / `[cancel]` / `[clear]` actions. Editing happens in place — no modal.

### Bottom bar

`-- running N --` if anything is in flight (else `-- idle --`), scheduled count, and a "last run" relative timestamp from the most recent successful run.

## User Flows

### Glance

Owner opens the app. Loading state is one line of dim text — no spinner. State arrives, sections render, status glyphs tell them which agents are scheduled, which ran today, which failed. Done.

### Run something

Click `[run]`. The status glyph immediately pulses cyan; the bottom bar shows `-- running 1 --`. When the run finishes, the glyph snaps to `✓` or `✗` and the row's "last run" updates.

### Schedule (or reschedule) an agent

Click the row (anywhere except `[run]`). The editor expands inline, prefilled with the current schedule (or sensible defaults: weekdays at 09:00). Adjust mode, days, time. The "next run" preview recomputes on every change. Click `[save]`. The row collapses, the schedule summary updates, the next-run countdown starts ticking. If the agent already had a schedule, `[clear]` removes it.

## Design Decisions

- **Density over breathing room.** PRODUCT.md design principle 3: "Bloomberg-terminal scan-ability." Rows are 1.5 leading, 12–14 px type, no card chrome, no rounded corners. Padding only where needed for hit targets.
- **Bracket buttons (`[run]`, `[save]`)** are deliberate: shell-verb voice, no rounded pill chrome, consistent with the retro-runtime brief. The brackets are part of the affordance — they say "this is interactive" the same way `[Y/n]` does in a CLI.
- **Inline editor, not modal.** Modals break the scan. Expanding under the row keeps the rest of the dashboard visible and contextual.
- **One schedule per agent.** A row is "scheduled" or "unscheduled" — binary. This matches the engine model and keeps the row visually decisive.
- **Status glyph encodes both state and recency.** The glyph reflects the *most recent* run's status, falling back to "scheduled / idle" if nothing has run yet. Color and shape are paired (per PRODUCT.md accessibility note) so palette swaps and color-blind users still parse it.
- **Theme name in the corner is interactive.** Functional retro: the label communicates state (which palette is loaded) *and* lets you act on it (reload). No decoration without function.
- **Times are tabular.** Anything numeric — counts, clocks, countdowns — uses `tabular-nums` so columns line up across rows.
- **No empty state copy.** PRODUCT.md design principle 5. If there are no scheduled agents the section just says `0 scheduled` — there is no "Get started!" card.
