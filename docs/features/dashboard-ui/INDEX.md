# Dashboard UI

The single screen of the app. A top bar (theme indicator + agent count + rescan), a body of agents grouped by section (Daily, Engineering, Reflection, Dev), and a bottom bar (running / scheduled / last-run summary). Each agent is a tight one-liner: status glyph, id, description, schedule summary, next-run countdown, run button. Clicking the row expands an inline schedule editor. State lives in a Zustand store that re-fetches on every `scheduler:changed` push from main.

## Documents

| Document | Purpose |
|----------|---------|
| [DESIGN.md](DESIGN.md) | Layout, density, status vocabulary, interaction model |
| [TECHNICAL.md](TECHNICAL.md) | Component tree, store, IPC integration, theming hooks |
