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
  - `service.ts` — thin AppService: lists agents/runs/missed, writes meta sidecars, spawns `aos refresh`, spawns `wrapper.sh` for manual runs
  - `cli.ts` — locates the `aos` binary on PATH and calls `aos home`
  - `agents/`, `scheduler/` (read-side helpers + meta-store), `theme/`, `ipc.ts`
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

- The app is a view. Anything that affects the user's system (cron, wrapper install, log trimming) lives in the `aos` CLI, not the renderer.
- If the user doesn't have `aos` on PATH, the dashboard renders a blocking banner and does nothing else.
- Filesystem first: new state is JSON under `<aos_home>/`, mutated by small focused code, rendered in the view. Reach for renderer logic only for genuinely interactive bits.
- Meta-sidecar writes (schedule, description) happen directly from the renderer, but the main process auto-invokes `aos refresh` afterwards so cron stays consistent.
- No hardcoded colors outside fallbacks — read from the active Omarchy theme.
- Terse labels, shell verbs, no emojis, no exclamation points.

## Documentation rule

When you add a feature, change a feature's behavior, or rename/move things users or agents reference, use the documentation skill.
Docs and code ship in the same change.
