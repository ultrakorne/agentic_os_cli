# Theme System — Design

## Overview

PRODUCT.md says "the OS theme is the soul." The app must look at home on gruvbox, tokyo-night, vantablack, rose-pine, or anything else the owner has installed in `~/.local/share/omarchy/themes/`. There is no in-app theme picker and no hardcoded palette outside fallbacks — the colors come from whichever Omarchy theme is currently active, and the app updates live when Omarchy switches.

## Components

### Theme payload

A `Theme` is `{ name, colors, source }`. `name` is whatever Omarchy reports (`tokyo-night`, `gruvbox`, …). `colors` is a flat record of 22 color slots: `bg`, `fg`, `accent`, `cursor`, `selection*`, and the 16-color terminal palette (`color0`–`color15`). `source` is `'omarchy'` if the file was found and parsed, `'fallback'` otherwise.

### Loader

Reads `~/.config/omarchy/current/theme/colors.toml` (a tiny subset of TOML — `key = "value"` lines). Missing keys fall back to the built-in tokyo-night-ish palette individually, so a partial or older Omarchy theme still works.

### Watcher

`fs.watch` on the colors file *and* the name file, debounced ~80 ms. Reload-and-broadcast on every fire. If either path doesn't exist, the watcher silently skips it — fallback theme is active and there is nothing to watch.

### Renderer side

`applyTheme` writes each color slot to a CSS variable on `<html>` (e.g. `--theme-bg`, `--theme-c5`). `styles.css` then maps those into semantic vars consumed by Tailwind's `@theme` (e.g. `--color-fg-dim`, `--color-success`, `--color-danger`). Components only ever reference the semantic vars — never the raw `--theme-*` slots.

## User Flows

### Theme switch in Omarchy

Owner runs `omarchy theme set gruvbox` (or whichever). `~/.config/omarchy/current/` is repointed; the watcher fires; the loader re-parses; the new `Theme` is broadcast to every window; `applyTheme` updates CSS variables; the UI repaints with the new palette without a reload.

### Manual reload

The `theme` label in the top bar is a button. Clicking it re-runs the loader and rebroadcasts. Useful when watching fails (different filesystem, symlink edge cases) or for development.

## Design Decisions

- **No in-app theme picker.** Theming is a system-level concern owned by Omarchy. Adding a picker would create two competing sources of truth.
- **Two-layer CSS contract.** `--theme-*` are the raw palette from disk; `--color-*` are the semantic tokens components use. This keeps components stable when palette interpretation changes (e.g. mapping which terminal color means "warn").
- **Fallback is built-in, not loaded from a file.** A user with no Omarchy install should still get a usable, on-brand UI immediately.
- **Per-key fallback, not whole-theme fallback.** If Omarchy ships a theme without an `accent` key, we fall back for *that key only*, not the whole palette. Future-proof against schema drift.
- **Color choice for status follows the terminal convention.** `color1`/red = danger, `color2`/green = success, `color3`/yellow = warn, `color4`/blue = info, `color6`/cyan = "cool" / running. These mappings live in `styles.css`.
