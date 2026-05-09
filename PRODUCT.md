# Product

## Register

product

## Users

A single owner: a developer running Omarchy on Linux (and occasionally macOS) who lives in terminals (vim, tmux, alacritty/kitty/ghostty). Comfortable with config files, shell, and keyboard-first workflows. Opens this app to launch personal agents, automations, and scripts, and to glance at what is scheduled to run today.

It is not a multi-tenant product. Other people may eventually run their own copy, but every design decision optimizes for the owner's daily use.

## Product Purpose

Agentic OS is a desktop dashboard that turns scattered agents, scripts, and skills into a single surface the owner can curate, click, and schedule. Sections group related work (Daily, Engineering, Reflection, etc.). Each agent can be triggered by click or by a schedule that catches up missed runs after the laptop was off. The point is to make the owner's automations feel like part of the OS, not a side project.

Success looks like the owner opening this every morning instead of remembering which scripts live where, and trusting that anything scheduled has either run, will run, or will catch up the next time the machine wakes.

## Brand Personality

Retro-runtime, deliberate, playful.

The aesthetic vocabulary is synthwave-CRT crossed with terminal-vim, expressed through modern UX, not nostalgia kitsch. Think WarGames-but-real, Cogmind, Norton Commander reimagined for 2026. Type and motion carry the personality. The color palette belongs to the *active Omarchy theme*, not to the app, so the same UI feels right whether the user is on gruvbox, tokyo-night, vantablack, or rose-pine.

Voice: terse, technical, slightly warm. Labels lean toward shell verbs over product copy. No explainer paragraphs. No emojis. No exclamation points.

## Anti-references

- SaaS dashboards and Notion clones: gradient cards, identical icon-and-title grids, generic "productivity" feel.
- Material design / default-Tailwind blue / Google-y rounded everything.
- Skeuomorphic or cartoon UIs: faux wood, mascots, jokey microcopy.
- Outrun-poster cliché: palm-tree silhouettes, sunset gradients, neon grid floors. The synthwave reference is sound and energy, not stock visual tropes.
- Cold corporate minimalism: all-grey, all-Helvetica, "everything is a card" Bauhaus reflex.

## Design Principles

1. **The OS theme is the soul.** Color, contrast, and surface mood come from the user's Omarchy palette at runtime via `~/.config/omarchy/current/theme/colors.toml`. The app must work, recognizably, across every theme in `~/.local/share/omarchy/themes/`. No hardcoded hex outside fallbacks.

2. **Type and layout carry the personality.** Monospace is the spine. The retro-runtime feel comes from typographic discipline, density, hierarchy, and motion, not from neon decoration.

3. **Density is power.** Show many agents at once. Tight leading, small type, no padding by reflex. Bloomberg-terminal scan-ability for the user's day.

4. **Functional retro.** Every CRT, glow, scanline, or boot-screen flourish earns its keep by communicating something (status, focus, history). No decoration for decoration's sake.

5. **Single-owner tool, no ceremony.** The dashboard is the product. No onboarding, no empty-state hand-holding, no "welcome" page. If state is empty, show what to do next inline.

## Accessibility & Inclusion

Single-user app, owner has no specific accessibility constraints. Soft defaults: respect `prefers-reduced-motion` for ambient animations (CRT flicker, scanline drift), pair color-coded status with text or shape (so color-blindness in any future theme does not break it). No formal WCAG target.
