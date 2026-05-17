# agent-management

Discover the agent scripts under `<aos_home>/agents/`, read their sidecar metadata, and edit it. Covers `aos list`, `aos describe`, and `aos schedule` — the three verbs that work against the agent + sidecar pair and never spawn the wrapper.

## Documents

| Document | Purpose |
|----------|---------|
| [DESIGN.md](DESIGN.md) | What "an agent" looks like to the CLI, the sidecar contract, scan rules, schedule input forms |
| [TECHNICAL.md](TECHNICAL.md) | Source-file map, scanner rules, schedule compilation, sidecar write semantics |
