# scheduler

Platform-backend reconciliation, missed-run detection, and stale-running sweep. Covers `aos refresh` (rescan agents, call `backend.Sync` to write/update/remove plists or unit files, record misses, sweep runs, trim log) and `aos tick` (one periodic cycle: detect misses, sweep stale running records, report backend drift, log a summary). The scheduler is where the system's "what time is it, what was supposed to fire, what hasn't?" logic lives.

## Documents

| Document | Purpose |
|----------|---------|
| [DESIGN.md](DESIGN.md) | What refresh and tick own, the missed-run / native-makeup model, drift detection, the stale-running sweep |
| [TECHNICAL.md](TECHNICAL.md) | Source-file map, backend file shapes (plist + systemd units), miss coverage rules, sweep semantics |
| [FLOW.mermaid](FLOW.mermaid) | One tick: detect → record → sweep stale → probe backend state → log |
