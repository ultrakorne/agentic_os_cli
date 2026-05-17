# interactive-tui — Technical

## Architecture

`aos start` (and the bare-`aos` fallback) constructs a Bubble Tea `startModel` composed of:

- one `bubbles/list.Model` per section, each with an item delegate (`agentDelegate`) that renders one row of glyph + id + "when" + schedule summary;
- a `bubbles/help.Model` whose binding set switches by `footerMode`;
- a `map[string]agentLocation` so fsnotify events update the matching `list.Item` without scanning every list;
- channels for `fsnotify.Event` and watcher errors, piped into the `tea.Program` via custom messages.

Pressing `Enter` constructs a `detailsModel` (full-screen) that composes `textarea`, `textinput`, `table`, `viewport`, and `help`. The parent model holds a pointer to the popup; while non-nil, `Update` routes input through the popup and `View` renders only its content. Saves in the popup go through the same `WriteSchedule` / `WriteDescription` writers the CLI verbs use and dispatch a `tea.Cmd` that re-reads the agent so the parent grid reflects the change.

## Source Files

| File | Role |
|------|------|
| `cmd/aos/start.go` | `aos start` verb: load config, scan agents, read runs, set up `fsnotify`, run `tea.Program`; also installs the bare-`aos` fallback on `rootCmd.RunE` |
| `cmd/aos/start_model.go` | `startModel` (Bubble Tea model), `sectionPanel`, `agentItem`, `agentDelegate`, `keyMap` + footer modes, fsnotify message wiring |
| `cmd/aos/start_view.go` | Layout algorithm (`applyLayout`), `View()`, section box rendering, title/footer/toast composition, `statusGlyph`, `relativeFromNow` |
| `cmd/aos/start_test.go` | `startModel` tests: layout sizing, focus, filter routing, fsnotify-driven updates |
| `cmd/aos/start_details.go` | `detailsModel` (popup model): config + history panes, schedule editor, run-output viewport |
| `cmd/aos/start_details_test.go` | Popup tests: schedule round-trip, save path, history pane render |
| `cmd/aos/style.go` | Shared lipgloss palette, `banner`, `printKV`, `newTable`, `statusStyle` — reused across CLI verbs and TUI |
| `cmd/aos/format.go` | `formatStartedAt`, `sanitize`, `agentRecord` — TUI uses `formatStartedAt` for the history table |
| `internal/scheduler/runs.go`, `meta_store.go`, `lookup.go`, `spawn.go` | Read paths and write paths shared with the CLI verbs (no TUI-specific scheduler code) |

## Data Model

The TUI doesn't introduce a new persisted shape — every model in memory derives from on-disk `Run` and `Agent` records. The `startModel` holds:

```go
type startModel struct {
    aosHome, runsDir, wrapper string
    sections []sectionPanel             // per-section list.Model + name + scheduledCount
    focused  int
    agentLoc map[string]agentLocation   // agent id -> (section idx, item idx)
    help     help.Model
    keys     keyMap
    width, height int
    toast    string                     // transient feedback row
    toastUntil time.Time
    events chan fsnotify.Event          // wired into tea via custom msg
    errs   chan error
    popup *detailsModel                 // non-nil while details overlay is open
}
```

`agentItem` (the `list.Item`) carries `scheduler.Agent` + the agent's latest `scheduler.Run`. `FilterValue()` is the agent id so `list.DefaultFilter` narrows by id.

## Noteworthy Behavior

- **fsnotify drives row updates, not a polling loop.** A new/renamed file under `runs/` raises an event that maps to an agent via `agentLoc`; the matching `list.Item` is rebuilt with the latest run and the list is re-set. No periodic re-scan — re-scans happen only on layout changes or popup-driven meta updates.
- **The popup is a separate model, not a sub-view.** While `m.popup != nil`, `Update` short-circuits and routes events through the popup; `View` returns only `popup.View()` with `AltScreen=true`. This avoids z-order / transparent-overlay complexity and matches the plan's "detail overlay (full-screen take-over)" wording.
- **Section focus is communicated to the delegate per render.** A list's `Index()` always points somewhere; the per-row `Render` only draws a selected highlight when `sectionFocused` is true. `refreshDelegates` re-installs each section's delegate before every `View` with the current focus flag.
- **Layout scales heights proportionally when content overflows.** `applyLayout` first asks each section for its natural height (`len(items) + chrome`); if the total fits, every section gets exactly what it asked for. If not, available rows are split proportionally to natural demand with a 1-row minimum per section, and `list.Model` handles its own internal scroll.
- **Filter prompt is hoisted out of the box.** `bubbles/list`'s built-in filter row is hidden in `newStartModel` so it doesn't shove an empty row into every section's bordered box. When the focused list enters filter mode, the prompt is re-rendered as a single line above the footer.
- **Help footer mode switches based on UI state.** `keys.mode` is set per-render: `footerModeEmpty` (no agents), `footerModeFiltering` (active filter on the focused section), `footerModeMain` (default). `ShortHelp()` returns a different binding set per mode.
- **Toast is rendered at parent level with a TTL.** `toastUntil` is set to `now + toastTTL`; a `tea.Tick` posts `clearToastMsg` after the TTL. Toast contents are styled with `styleWarn` so they're visible against the section grid.
- **Manual run dispatch reuses the scheduler spawn path.** `x` calls the same `SpawnWrapperDetached` the CLI's `aos run` uses with `TRIGGER=manual` and a fresh `NewRunID`; the stub isn't printed (no stdout in the TUI) — the dashboard finds out about the new run via fsnotify like everything else.
- **Saves in the popup write the sidecar then re-scan.** `WriteSchedule` / `WriteDescription` are the same writers `aos schedule` / `aos describe` use; the popup dispatches an `agentMetaUpdatedMsg` so the parent grid re-reads the row. A toast confirms the save.
- **History viewport loads `.out` off the goroutine.** Reading a potentially-large run record happens in a goroutine that posts a `runOutputLoadedMsg` back to the popup; `Update` is never blocked on disk I/O.
- **`aos` (no args) falls through to start via `rootCmd.RunE`.** Cobra still routes `aos help`, `aos -h`, and known verbs to their dedicated commands; unknown verbs hit cobra's built-in "unknown command" error before reaching `runStart`.

## Dependencies

- `charm.land/bubbletea/v2` — the program loop.
- `charm.land/bubbles/v2` — `list`, `help`, `key`, `table`, `viewport`, `textarea`, `textinput`, `paginator`, `spinner`.
- `charm.land/lipgloss/v2` — styling and box composition.
- `github.com/fsnotify/fsnotify` — live updates from `runs/`.
- `internal/scheduler` — scan, run read, sidecar write, wrapper spawn.
- `internal/config` — `aos_home`.
