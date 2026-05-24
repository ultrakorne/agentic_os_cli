# lifecycle

Install, relocate, and uninstall the aos runtime on a user's machine. Covers `aos init`, `aos home`, and `aos uninstall` — the verbs that own `~/.config/aos/config.toml`, the embedded `wrapper.sh`, the `agents/` and `runs/` subdirectories, and (in the uninstall path) the platform backend's managed namespace (LaunchAgents on macOS / systemd-user units on Linux).

## Documents

| Document | Purpose |
|----------|---------|
| [DESIGN.md](DESIGN.md) | What the three verbs do, what user-data they preserve, relocation semantics |
| [TECHNICAL.md](TECHNICAL.md) | Source-file map, embedded-asset contract, relocation algorithm, install script |
