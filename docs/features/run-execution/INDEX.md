# run-execution

Manual run dispatch and run-record inspection. Covers `aos run` (fires a detached `wrapper.sh`, optionally blocks with `--wait`) and `aos runs` (list or show), plus the shared run/wait/spawn primitives in `internal/scheduler/`. Backend-driven scheduled runs are dispatched by launchd / systemd-user (see the [scheduler](../scheduler/INDEX.md) feature), but every run — scheduled or manual — reads and writes the same `<aos_home>/runs/` directory.

## Documents

| Document | Purpose |
|----------|---------|
| [DESIGN.md](DESIGN.md) | What manual runs look like, how `--wait` overlays progress without breaking stdout |
| [TECHNICAL.md](TECHNICAL.md) | Source-file map, wrapper argv contract, run-id format, stub-vs-record correlation |
