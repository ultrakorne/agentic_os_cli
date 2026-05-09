# AGENTS.md

Electron dashboard for clickable and scheduled agents. Single owner, retro-runtime aesthetic.

## Start here

- [docs/INDEX.md](docs/INDEX.md) — architecture, tech stack, and feature map. Read before changing anything non-trivial.
- Feature docs live under `docs/features/<feature>/` (`INDEX.md`, `DESIGN.md`, `TECHNICAL.md`).

## Architecture in one line

A view over filesystem state: system cron fires `wrapper.sh`, which writes JSON + `.out` files under `<userData>/data/runs/`; the renderer reads them. See `docs/INDEX.md` for the full contract.

## Code layout

- `src/main/` — Electron main: `agents/`, `scheduler/`, `theme/`, `ipc.ts`
- `src/preload/` — context bridge
- `src/renderer/src/` — React 19 + Zustand + Tailwind 4 dashboard
- `src/cli/tick.ts` — out-of-process engine tick (`pnpm tick`)
- `src/shared/` — types shared across processes
- Tests live next to code as `*.test.ts` (Vitest)

## Commands

```
pnpm dev          # run app
pnpm test         # vitest run
pnpm typecheck    # node + web
pnpm lint
pnpm tick         # run engine tick once
```

## Conventions

- Filesystem first: new state is JSON under `<userData>/data/`, mutated by small focused code, rendered in the view. Reach for renderer logic only for genuinely interactive bits.
- If a piece of work can run from cron without the app open, it should.
- No hardcoded colors outside fallbacks — read from the active Omarchy theme.
- Terse labels, shell verbs, no emojis, no exclamation points.

## Documentation rule

When you add a feature, change a feature's behavior, or rename/move things users or agents reference, use the documentation skill.
Docs and code ship in the same change.
