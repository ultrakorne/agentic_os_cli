# AGENTS.md

Electron dashboard for clickable and scheduled agents. Single owner, retro-runtime aesthetic.

## Start here

- [docs/INDEX.md](docs/INDEX.md) — architecture, tech stack, and feature map. Read before changing anything non-trivial.
- Feature docs live under `docs/features/<feature>/` (`INDEX.md`, `DESIGN.md`, `TECHNICAL.md`).
- The `aos` CLI is a sibling repo at [`agentic_os_cli/`](agentic_os_cli/AGENTS.md). It owns the runtime — wrapper, crontab block, scheduler tick.

## Architecture in one line

A view over filesystem state: the `aos` CLI installs `wrapper.sh` and a managed cron block; cron fires the wrapper, which writes JSON + `.out` files under `<aos_home>/runs/`; the Electron renderer reads them.

## Code layout

- `src/main/` — Electron main:
  - `service.ts` — thin AppService: caches the agent list, shells out to `aos list/schedule/describe/refresh/run` for every mutation, returns the JobRun stub from `aos run` for manual launches (wrapper-spawn lives in the CLI)
  - `cli.ts` — locates the `aos` binary on PATH and calls `aos home --json`
  - `exec.ts` — shared `execCapture(bin, args)` child-process helper
  - `agents/agent-list.ts` — parses `aos list --json` into the renderer's `Agent[]`; also formats `aos schedule` flags
  - `scheduler/runs-store.ts` — watches `<aos_home>/runs/` and asks `aos runs --json --limit N` for the snapshot; `.out` reads stay as plain `fs.readFile` (view-only). Missed scheduled slots show up here as `JobRun{status:"missed"}` entries — there is no separate misses store (see `agentic_os_cli/MISSES_AS_RUNS_PLAN.md`)
  - `theme/`, `ipc.ts`
- `src/preload/` — context bridge
- `src/renderer/src/` — React 19 + Zustand + Tailwind 4 dashboard
- `src/shared/` — types shared across processes
- Tests live next to code as `*.test.ts` (Vitest)

## Commands

```
pnpm dev          # run app (requires `aos` on PATH)
pnpm test         # vitest run
pnpm typecheck    # node + web
pnpm lint
```

## Conventions

- The app is a view. Anything that affects the user's system (cron, wrapper install, log trimming, sidecar writes, scanning) lives in the `aos` CLI, not the renderer.
- **Every `aos` invocation passes `--json`.** The JSON shape is the contract between the CLI and this app (see `agentic_os_cli/AGENTS.md` "Output: humans vs. clients"). Never parse the human-formatted output — it's styled with lipgloss for terminals and is free to change. New CLI calls must also use `--json`; if a new verb needs a JSON branch and doesn't have one, add it to the CLI before wiring it up here.
- If the user doesn't have `aos` on PATH, the dashboard renders a blocking banner and does nothing else.
- Filesystem first: new state is JSON under `<aos_home>/`, mutated by small focused code, rendered in the view. Reach for renderer logic only for genuinely interactive bits.
- Schedule and description edits shell out to `aos schedule` / `aos describe`. The CLI writes the meta sidecar **and** reconciles cron; the main process re-pulls `aos list --json` to refresh the cache. There is no TS-side scanner or meta-store anymore.
- Run listings shell out to `aos runs --json` (the watcher only debounces, the CLI parses + sorts + limits). Manual launches shell out to `aos run --json` and use the JobRun stub it prints — there is no TS-side wrapper.sh invocation.
- No hardcoded colors outside fallbacks — read from the active Omarchy theme.
- Terse labels, shell verbs, no emojis, no exclamation points.

## Documentation rule

When you add a feature, change a feature's behavior, or rename/move things users or agents reference, use the documentation skill.
Docs and code ship in the same change.
