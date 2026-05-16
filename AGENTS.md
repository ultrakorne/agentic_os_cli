# AGENTS.md

Go CLI (`aos`) that installs and manages the agentic_os runtime: writes `wrapper.sh` into the user's home, reconciles the crontab from agent definitions, and ticks the scheduler.

## Code layout

- `cmd/aos/` — cobra subcommands (`init`, `list`, `describe`, `schedule`, `run`, `runs`, `refresh`, `tick`, `uninstall`, `home`) plus shared package-local helpers in `format.go` (`sanitize`, `agentRecord`) and `style.go` (lipgloss palette, `printJSON`, `printKV`, `newTable`, `banner`, `statusStyle`)
- `internal/config/` — paths, env, and config resolution
- `internal/resources/` — files embedded into the binary at build time (`wrapper.sh` via `//go:embed`)
- `internal/runtime/`, `internal/scheduler/`, `internal/crontab/`, `internal/logtrim/` — building blocks the commands wire together
- Tests live next to code as `*_test.go` (standard library `testing`)

## Commands

```
go build ./...              # compile every package
go test ./...               # run all tests
go vet ./...                # static checks
```

Install procedure for end users lives in `README.md`.

## Conventions

- Embedded assets are the source of truth — `aos init` overwrites the on-disk copy if it drifts from `resources.WrapperSh`.
- Cross-platform paths via `filepath.Join`; never hardcode separators.
- Errors wrap with `fmt.Errorf("context: %w", err)` so callers can `errors.Is`/`As`.
- Keep shared package-local helpers (e.g. `sanitize`) in a neutral file like `format.go`, not in a command-specific file.
- Every verb supports a persistent `--json` flag (`JSONOutput()`); machine output goes through `printJSON` for one indent rule everywhere. Human output uses lipgloss via the `style.go` helpers (`printKV`, `newTable`, `banner`, `statusStyle`) — never inline ANSI escapes. Lipgloss strips styling when stdout isn't a TTY, so piping stays clean.
- The JSON shape is the contract for clients (Electron, agents). Restyle the human output freely, but don't change `--json` field names/types without bumping the consumers.
