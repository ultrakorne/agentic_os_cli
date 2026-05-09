# Dashboard UI — Technical

## Architecture

Standard Electron renderer: `main.tsx` mounts `<App />`. `App` reads from a single Zustand store (`useApp`) and renders three blocks — top bar, `<Dashboard />`, bottom bar. `Dashboard` groups agents by section and renders one `<Section>` + many `<AgentRow>`s. Each `AgentRow` owns its expand state via a single `expandedId` lifted to `Dashboard` so only one editor is open at a time.

The store hydrates by calling three IPC methods in parallel (`agents.list`, `scheduler.listSchedules`, `scheduler.listRuns`) and subscribes to two pushes from main: `theme:changed` and `scheduler:changed`. Every scheduler change triggers a full re-fetch — the data is small and the engine is the source of truth, so there is no client-side optimistic state.

Theme bridges main → renderer via `applyTheme`, which writes `--theme-*` CSS vars on `<html>`. `styles.css` derives semantic `--color-*` tokens. Components reference only the semantic tokens (e.g. `text-[var(--color-fg-dim)]`, `bg-[var(--color-accent)]`).

## Source Files

| File | Role |
|------|------|
| `src/renderer/index.html` | Vite entry HTML; mounts `#root` |
| `src/renderer/src/main.tsx` | React 19 root; mounts `<App />` and imports `styles.css` |
| `src/renderer/src/App.tsx` | Top bar + `<Dashboard />` + bottom bar; computes header/footer stats from store |
| `src/renderer/src/store.ts` | Zustand store: agents, schedules (Map by jobId), runs, theme; `init()` wires IPC subscriptions; exports `SECTION_ORDER` |
| `src/renderer/src/theme.ts` | `applyTheme`: writes `Theme.colors` to `--theme-*` CSS vars + `data-theme-*` on `<html>` |
| `src/renderer/src/styles.css` | Tailwind 4 entry + `@theme` block deriving semantic `--color-*` tokens; scrollbar + pulse animation |
| `src/renderer/src/dashboard/Dashboard.tsx` | Groups agents by section; owns the single-expanded-row state |
| `src/renderer/src/dashboard/Section.tsx` | Section header (title + counts + dotted rule) wrapping a list of rows |
| `src/renderer/src/dashboard/AgentRow.tsx` | Row UI + run button + status glyph; renders `<ScheduleEditor>` when expanded |
| `src/renderer/src/dashboard/ScheduleEditor.tsx` | Inline editor: mode toggle, hourly/daily controls, live next-run preview, save/cancel/clear |
| `src/renderer/src/lib/format.ts` | `describeSpec`, `describeSchedule`, `relativeFromNow`, `formatClock` — pure formatters |
| `src/renderer/src/env.d.ts` | Ambient declaration for `window.api` typed as `AppAPI` |
| `src/preload/index.ts` | The `window.api` surface — agents, scheduler, theme |
| `src/preload/index.d.ts` | Type augmentation so the renderer sees `window.api` and `window.electron` |

## Data Model

The renderer stores agents as a flat array, schedules as a `Map<jobId, Schedule>`, runs as a flat array. Schedule lookup by jobId is O(1) for row rendering; the runs array is scanned once to build a `Map<jobId, JobRun>` of "most recent run per job" inside `Dashboard` via `useMemo`.

Section ordering uses a static array (`SECTION_ORDER = ['Daily', 'Engineering', 'Reflection', 'Dev']`); any section not in the list appears after, in the order encountered. There is no UI to reorder sections.

## Noteworthy Behavior

- **One open editor at a time.** `expandedId` lives in `<Dashboard>`; clicking another row swaps which row is expanded. Clicking the same row toggles closed.
- **Re-fetch on every scheduler change, no diffing.** The store calls `refresh()` on every `scheduler:changed`. Cheap because everything is in-process IPC and the data is tiny. Avoids a class of "stale view" bugs at the cost of redrawing.
- **Next-run preview is server-computed.** Both `AgentRow` (for the row's countdown) and `ScheduleEditor` (for the live preview while editing) call `window.api.scheduler.nextRun(spec)`. Croner lives only in the main process; the renderer never depends on it. `useEffect` cancels stale promises with a `cancelled` flag.
- **Status glyph derives from "most recent run".** `Dashboard` builds `lastRunByJob` from the runs array; the glyph shown is that run's status (running / success / error). If no run exists, the glyph falls back to scheduled (`◇`) vs idle (`·`).
- **`pulse-soft` honors `prefers-reduced-motion`.** `styles.css` disables the running-glyph pulse when the OS asks for reduced motion. Same handling will apply to any future ambient animation.
- **Stepper buttons are intentionally not number inputs.** Native `<input type="number">` on Linux Electron has inconsistent stepping behavior and is wrong for the aesthetic. The custom `Stepper` clamps to min/max and supports a `step` (used to jump minutes by 5).
- **Day toggle uses `aria-pressed` and color+shape pairing** so the active state is conveyed without relying on color alone. (M/T/W/T/F/S/S labels are intentional — letters are unique enough in column.)
- **The `scheduler.onChange` listener returns an unsubscribe function** that the store collects in `init()` and the App component calls on unmount. Same pattern for `theme.onChange`. Both are wired through `ipcRenderer.on/off` in the preload.

## Dependencies

- `react` 19, `react-dom` 19
- `zustand` 5 — single store
- `tailwindcss` 4 — utility-first styling, CSS-first `@theme`
- `window.api` (preload bridge) — IPC surface; defined in `src/preload/index.ts`
