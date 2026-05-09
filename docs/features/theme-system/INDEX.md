# Theme System

The app does not own its color palette — Omarchy does. At launch the main process reads `~/.config/omarchy/current/theme/colors.toml` plus `theme.name`, ships the result to the renderer, and `applyTheme` writes the colors as CSS custom properties on `<html>`. A watcher reloads the theme when Omarchy switches it. A built-in fallback (Tokyo-night-ish) keeps the app usable on systems without Omarchy.

## Documents

| Document | Purpose |
|----------|---------|
| [DESIGN.md](DESIGN.md) | Why theming is owned by the OS, how palette decisions are made |
| [TECHNICAL.md](TECHNICAL.md) | File paths, parser, watcher, CSS-variable contract |
