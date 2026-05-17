# scheduler

Crontab reconciliation, missed-run detection, and catch-up dispatch. Covers `aos refresh` (rescan agents + rewrite the managed cron block + record misses + sweep runs + trim log) and `aos tick` (one cron-driven cycle: detect misses, fire catch-ups, log a summary). The scheduler is where the system's "what time is it, what was supposed to fire, what hasn't?" logic lives.

## Documents

| Document | Purpose |
|----------|---------|
| [DESIGN.md](DESIGN.md) | What refresh and tick own, missed-vs-catchup model, drift detection |
| [TECHNICAL.md](TECHNICAL.md) | Source-file map, cron block format, miss coverage rules, catch-up gate |
| [FLOW.mermaid](FLOW.mermaid) | One tick: detect → record → catch-up → log |
