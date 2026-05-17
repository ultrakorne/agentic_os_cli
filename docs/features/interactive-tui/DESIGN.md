# interactive-tui — Design

## Overview

`aos start` opens an interactive terminal dashboard. It scans the agents directory, groups agents into per-section boxes, watches `<aos_home>/runs/` with fsnotify for live updates, and routes keystrokes through Bubble Tea. The screen has two layers: the section grid (always rendered) and an optional full-screen details popup (opened with Enter on an agent row). Bare `aos` with no arguments is an alias for `aos start`.

## Components

### The section grid (main view)

One bordered box per section (recovered from the agent's parent directory). Each box is a `bubbles/list` of agent rows; one section is *focused* at a time and renders with the accent border. Per-row: a status glyph (running ●, success ◆, error ▲, missed ▽, scheduled but never ran ◇, unscheduled ·), the agent id, a relative "when" timestamp ("just now", "3h ago"), and the schedule summary. Box widths are uniform across sections so the layout is visually steady regardless of content; heights default to one row per item, scaling down proportionally with internal scroll when the screen can't fit everything.

### Header and footer

The top line shows `aos start` (left, accent) and the `aos_home` path (right, muted). The bottom line is a `bubbles/help` footer whose binding set changes with mode: main, filtering, or empty state. A toast row sits between content and footer when transient feedback is showing (e.g. after `x` runs an agent).

### Filter

Pressing `/` activates the focused list's internal filter (`bubbles/list`'s `DefaultFilter` fuzzy matcher over agent id). The filter prompt is hoisted out of the section box and rendered as a single line just above the footer so it doesn't push every other section's content around. `Esc` clears the filter; `Enter` keeps the narrowed view.

### Run dispatch

`x` on the focused row spawns the agent in the background (same path as `aos run`). A toast confirms; the new run record materializes via fsnotify within a beat and the row's glyph flips to `running`.

### The details popup (full-screen overlay)

`Enter` opens a popup that owns the whole screen. Two tabs:

- **config** — the description (multiline `textarea`), schedule kind pills (off / hourly / daily), and kind-specific fields (`every-hours`, `hour`, `minute`, days chips). Save with `Ctrl+S`; navigation moves focus through fields; `e` or `Enter` enters edit mode on the focused field. Saves go through the same sidecar writers (`WriteSchedule` / `WriteDescription`) the CLI verbs use, so any change there is reflected in the dashboard's grid and in `aos list`.
- **history** — a `bubbles/table` of past runs (newest first) with a `bubbles/viewport` rendering the selected run's record + captured `.out`. `Enter` on a row opens the run's output in the viewport.

`Tab` cycles tabs; `1` / `2` jump to a specific tab; `Esc` closes the popup.

## User Flows

### Browse and run an agent

1. User runs `aos start` (or just `aos`).
2. The grid renders with every agent grouped by section; one section is focused.
3. User presses `j`/`k` to move within a section, `1`–`9` to jump between sections.
4. User presses `x` on the desired agent; a wrapper is spawned and a toast confirms.
5. The agent row's glyph flips to `running` within ~100 ms as fsnotify picks up the new file.

### Filter to find an agent

1. User presses `/`.
2. Filter prompt appears above the footer; typing narrows the focused list by id (fuzzy match).
3. `Enter` keeps the narrowed view; `Esc` clears the filter.

### Edit a schedule from the dashboard

1. User presses `Enter` on an agent row to open the details popup on the config tab.
2. User navigates to the kind pills, switches to `daily`, fills `hour`/`minute`, toggles the days chips.
3. `Ctrl+S` writes the sidecar atomically; an in-process refresh runs so the managed crontab reflects the change.
4. A toast confirms; the popup stays open. `Esc` returns to the grid, where the agent's row now shows the new schedule summary.

### Inspect run history

1. From the details popup, user presses `Tab` (or `2`) to switch to the history tab.
2. The table shows recent runs newest first; arrow keys move the cursor.
3. `Enter` loads the selected run's `.out` into the viewport on the right.

## Design Decisions

- **Composes bubbles primitives rather than hand-rolling.** Per-AGENTS.md: tables go through `bubbles/list`, the help footer through `bubbles/help`, the filter prompt through list's built-in filter, the editor through `bubbles/textarea`/`textinput`, the history viewer through `bubbles/table`/`viewport`. The only bespoke widgets are the schedule kind pills and the days chips (no radio / multi-toggle in bubbles at the time of writing).
- **Full-screen takeover for details, not an overlay.** A z-order overlay would force transparent rendering and per-frame compositing math; full-screen takeover keeps the renderer simple and matches the dashboard's "one thing at a time" UX.
- **fsnotify on `runs/`, not on `agents/`.** Run records change frequently (new run every minute, missed records on tick); agent files change rarely (user adds a script). Watching only `runs/` is the bigger value; the next `aos start` re-scans agents from scratch.
- **Section focus highlight is per-row, not just per-box.** A list's `Index()` always points at *some* item, but only the active section should draw its row as "selected" — otherwise every section keeps a stale accent on whatever the cursor last sat on. The delegate is re-installed per render with a `sectionFocused` flag.
- **Layout adapts to content size, then to screen size.** A 2-agent section stays 2 rows tall instead of inflating to a third of the screen. Only when the natural sum overflows do all sections scale down proportionally; lists handle their own scroll past that point.
- **Help footer changes with mode.** Main, filtering, and empty-state bindings differ — the footer reflects only what's currently legal so the operator isn't reading bindings for the wrong mode.
- **`aos` with no args opens the TUI.** Cobra routes known verbs, `--help`, and unknown verbs to their proper handlers; the bare-args fallback only triggers when no subcommand is named. Discoverable without surprising scripts.
