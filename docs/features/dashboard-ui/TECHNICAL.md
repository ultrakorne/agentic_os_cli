# Dashboard UI — Technical

## Architecture

Standard Electron renderer: `main.tsx` mounts `<App />`. `App` reads from a single Zustand store (`useApp`) and renders three blocks — top bar, `<Dashboard />`, bottom bar. `Dashboard` groups agents by section and renders one `<Section>` + many `<AgentCard>`s. Selecting a card opens `<ScheduleEditor>` in a centered overlay; only one editor is open at a time.

The store hydrates by calling four IPC methods in parallel (`agents.list`, `scheduler.listRuns`, `scheduler.listMissed`, `scheduler.crontabStatus`) and subscribes to two pushes from main: `theme:changed` and `scheduler:changed`. Every scheduler change triggers a full re-fetch — the data is small and the engine is the source of truth, so there is no client-side optimistic state.

Theme bridges main → renderer via `applyTheme`, which writes `--theme-*` CSS vars on `<html>`. `styles.css` derives semantic `--color-*` tokens. Components reference only the semantic tokens (e.g. `text-[var(--color-fg-dim)]`, `bg-[var(--color-accent)]`).

## Source Files

| File | Role |
|------|------|
| `src/renderer/index.html` | Vite entry HTML; mounts `#root` |
| `src/renderer/src/main.tsx` | React 19 root; mounts `<App />` and imports `styles.css` |
| `src/renderer/src/App.tsx` | Top bar + `<Dashboard />` + bottom bar; computes header/footer stats from store |
| `src/renderer/src/store.ts` | Zustand store: agents (each carries its schedule), runs, missed, crontabStatus, theme; `init()` wires IPC subscriptions; exports `SECTION_ORDER` |
| `src/renderer/src/theme.ts` | `applyTheme`: writes `Theme.colors` to `--theme-*` CSS vars + `data-theme-*` on `<html>` |
| `src/renderer/src/styles.css` | Tailwind 4 entry + `@theme` block deriving semantic `--color-*` tokens; scrollbar + pulse animation |
| `src/renderer/src/dashboard/Dashboard.tsx` | Groups agents by section; renders SystemBanner + MissedRunsBanner + cards; opens the editor overlay |
| `src/renderer/src/dashboard/Section.tsx` | Section header (title + counts + dotted rule) wrapping a card grid |
| `src/renderer/src/dashboard/AgentCard.tsx` | Card UI + run button + status glyph + missed-count badge + inline launch-error display |
| `src/renderer/src/dashboard/SystemBanner.tsx` | Top-of-page banner for missing system deps (crontab/python3/wrapper), crontab conflict, non-executable agent scripts |
| `src/renderer/src/dashboard/MissedRunsBanner.tsx` | Top-of-page banner showing recent missed runs with per-row "run now" buttons |
| `src/renderer/src/dashboard/ScheduleEditor.tsx` | Modal overlay editor: mode toggle, hourly/daily controls, live next-run preview, save/cancel/clear |
| `src/renderer/src/lib/format.ts` | `describeSpec`, `describeSchedule`, `relativeFromNow`, `formatClock` — pure formatters |
| `src/renderer/src/env.d.ts` | Ambient declaration for `window.api` typed as `AppAPI` |
| `src/preload/index.ts` | The `window.api` surface — agents, scheduler, theme |
| `src/preload/index.d.ts` | Type augmentation so the renderer sees `window.api` and `window.electron` |

## Data Model

The renderer stores agents as a flat array (each `Agent` carries its own `schedule`), runs as a flat array, missed runs as a flat array, and the latest crontab status. There is no separate schedules collection — the schedule lives on the agent. The runs array is scanned once inside `Dashboard` via `useMemo` to build a `Map<jobId, JobRun>` of "most recent run per agent".

Section ordering uses a static array (`SECTION_ORDER = ['Agents', 'Daily', 'Engineering', 'Reflection', 'Dev']`); any section not in the list appears after, in the order encountered. Sections themselves come from each agent's parent directory at scan time — there is no UI to reorder them, the user reorganizes by `mv`.

## Noteworthy Behavior

- **One open editor at a time.** `selectedId` lives in `<Dashboard>`; clicking a different card swaps which is selected and opens its editor in a centered overlay. Clicking the same card or pressing Escape closes the overlay.
- **Re-fetch on every scheduler change, no diffing.** The store calls `refresh()` on every `scheduler:changed`. Cheap because everything is in-process IPC and the data is tiny. Avoids a class of "stale view" bugs at the cost of redrawing.
- **Top-bar `rescan` is the manual discovery trigger.** The agents directory is otherwise scanned only at `engine.start()` and on the existing `EmptyAgents` / `SystemBanner` rescan affordances — nothing watches it live, and the 5-minute sweep is missed-run only. The top-bar button calls `agents:rescan`, which re-runs `engine.refreshScripts()` (re-reading every `<id>.meta.json` sidecar) and broadcasts `scheduler:changed`. Useful after dropping a new script, hand-editing a sidecar, or moving an agent between section folders.
- **Next-run preview is server-computed.** Both `AgentCard` (for the card's countdown) and `ScheduleEditor` (for the live preview while editing) call `window.api.scheduler.nextRun(spec)`. Croner lives only in the main process; the renderer never depends on it. `useEffect` cancels stale promises with a `cancelled` flag.
- **Status glyph derives from "most recent run".** `Dashboard` builds `lastRunByJob` from the runs array; the glyph shown is that run's status (running / success / error). If no run exists, the glyph falls back to scheduled (`◇`) vs idle (`·`).
- **Missed runs appear inline in the detail run history.** When a card's detail view is open, `RunHistoryList` filters the store's `missed` slice for that agent and merges it with the run history, sorted by `expectedAt` / `startedAt`. Missed entries render via `MissedRunRow` (dashed border, `▽` glyph, `missed` trigger badge) — read-only, no controls. They self-clear when `detectMissed` stops reporting the slot (e.g. once a manual run covers it via the banner's "run now").
- **Manual-run errors show inline on the card.** If `runNow` returns a stub with `status === 'error'` (wrapper missing, python3 missing, no script), the card surfaces the message under its description for ~5s, then auto-clears. The runtime errors that would surface only as scheduled-run failures are caught the same way through the runs file watcher.
- **Reconcile is two-click.** The crontab-conflict warning's button changes its label to `confirm reconcile?` on first click; second click within 4s commits the destructive overwrite.
- **`pulse-soft` honors `prefers-reduced-motion`.** `styles.css` disables the running-glyph pulse when the OS asks for reduced motion. Same handling will apply to any future ambient animation.
- **Stepper avoids native `<input type="number">`** because Linux Electron has inconsistent stepping behavior and it's wrong for the aesthetic. The custom `Stepper` uses a plain text input with `inputMode="numeric"` so the value is keyboard-editable, supports a `step` (used to jump minutes by 5), and the `+` / `−` buttons (and ArrowUp/ArrowDown) wrap around — incrementing past `max` lands on `min`, decrementing past `min` lands on the highest reachable step. Out-of-range typed values are clamped on blur.
- **Day toggle uses `aria-pressed` and color+shape pairing** so the active state is conveyed without relying on color alone. (M/T/W/T/F/S/S labels are intentional — letters are unique enough in column.)
- **The `scheduler.onChange` listener returns an unsubscribe function** that the store collects in `init()` and the App component calls on unmount. Same pattern for `theme.onChange`. Both are wired through `ipcRenderer.on/off` in the preload.

## Dependencies

- `react` 19, `react-dom` 19
- `zustand` 5 — single store
- `tailwindcss` 4 — utility-first styling, CSS-first `@theme`
- `window.api` (preload bridge) — IPC surface; defined in `src/preload/index.ts`
