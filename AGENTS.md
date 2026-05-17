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

## Output: humans vs. clients

Every verb has two output modes, and **every new verb must implement both**:

- **Human (default)** — styled with [lipgloss](https://github.com/charmbracelet/lipgloss) for a clean terminal view: rounded-border tables for listings, right-aligned key/value blocks for single records, colored status / health fields, accent-banner per command. Never write raw `tabwriter` rows or inline ANSI escapes — use the shared helpers in `cmd/aos/style.go`: `banner`, `printKV`, `newTable`, `statusStyle`, plus the `style*` / `color*` palette. Lipgloss auto-detects the terminal's color profile via termenv and strips styling when stdout isn't a TTY, so piping/redirecting still produces clean text.
- **`--json` (clients and agents)** — a persistent root flag (`JSONOutput()`); the Electron app and any scripted agent consumes this. Every `--json` branch must go through `printJSON` so indentation and trailing-newline behavior stay uniform. **The JSON shape is the contract**: restyle the human output freely, but don't rename or retype `--json` fields without bumping the consumers (Electron `src/shared/scheduler.ts`, agent integrations).

When adding a new verb, the runner ends with `if JSONOutput() { return printJSON(payload) }; return printHumanFn(payload)` — never just one or the other.

## Conventions

- Embedded assets are the source of truth — `aos init` overwrites the on-disk copy if it drifts from `resources.WrapperSh`.
- Cross-platform paths via `filepath.Join`; never hardcode separators.
- Errors wrap with `fmt.Errorf("context: %w", err)` so callers can `errors.Is`/`As`.
- Keep shared package-local helpers (e.g. `sanitize`) in a neutral file like `format.go` or `style.go`, not in a command-specific file.
- Commands that take a required positional arg (`init`, `describe`, `schedule`, `run`) use `cobra.MaximumNArgs(N)` plus a `len(args) == 0 → cmd.Help()` short-circuit so `aos <verb>` prints usage instead of a terse "accepts 1 arg(s)" error.

## Reuse before building (TUI and CLI)

When building anything in `cmd/aos/` — TUI screens, key handling, list rendering, footers, help, tables, progress, spinners, filtering, status formatting — **first** look at what `charm.land/bubbletea/v2`, `charm.land/bubbles/v2` (`list`, `viewport`, `help`, `key`, `paginator`, `spinner`, `progress`, `textinput`, `table`), and `charm.land/lipgloss/v2` already provide. Prefer composing those over hand-rolling.

Do not introduce bespoke equivalents — custom scrollers, custom filter inputs, hand-drawn borders, manual key dispatchers, ad-hoc help footers — without asking first. Surface the choice to the human, name what's available in the framework, and explain why a bespoke piece is justified. They decide.

This applies to every file under `cmd/aos/`, not just the TUI: tables go through the shared helpers in `style.go`, JSON branches go through `printJSON`, status colors come from `statusStyle`, and so on.
