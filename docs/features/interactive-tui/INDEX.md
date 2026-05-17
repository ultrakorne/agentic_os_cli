# interactive-tui

The Bubble Tea dashboard rendered by `aos start` (also the default verb when `aos` is run with no arguments). Agents grouped by section with vim-style navigation, in-list filter, one-key manual runs, and a full-screen details popup with config (description + schedule editor) and history (past runs + their output). Live-updates from `fsnotify` as the wrapper writes new run records.

## Documents

| Document | Purpose |
|----------|---------|
| [DESIGN.md](DESIGN.md) | Layout, key bindings, the details popup's two tabs, live-update model |
| [TECHNICAL.md](TECHNICAL.md) | Source-file map, Bubble Tea model composition, fsnotify wiring, layout algorithm |
