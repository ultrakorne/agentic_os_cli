# lifecycle — Technical

## Architecture

Lifecycle is split across two layers. The verb layer in `cmd/aos/` parses flags, decides modes (`fresh|same|moved|repointed`), and emits human or JSON output; the persistence layer in `internal/config/` owns the TOML file and the `~/.config/aos/` directory. `wrapper.sh` is embedded into the binary at build time via `//go:embed` so a single binary install carries everything needed to bootstrap a home. Platform scheduler reconciliation is *not* implemented here — both `init` and `uninstall` delegate to `internal/scheduler/backend` (via the scheduler feature's `Refresh` and `Backend.Remove`).

## Source Files

| File | Role |
|------|------|
| `cmd/aos/init.go` | `aos init` verb: argument expansion, relocation, embedded-wrapper write, config merge, in-process refresh |
| `cmd/aos/init_linger_linux.go` | Linux-only interactive prompt: probes `XDG_SESSION_TYPE`, offers `sudo loginctl enable-linger $USER` when refresh reports linger disabled on a headless box |
| `cmd/aos/init_linger_other.go` | macOS/other-platform shim: `maybePromptLinger` no-ops |
| `cmd/aos/init_test.go` | Init tests: same-path no-op, fresh install, relocation (in-tree + EXDEV simulation), config merge |
| `cmd/aos/home.go` | `aos home` verb: read config, print path or `{"home": "<path>"}` |
| `cmd/aos/uninstall.go` | `aos uninstall` verb: orchestrates wrapper removal, tick.log removal, `backend.Remove`, config removal |
| `internal/config/config.go` | TOML schema (`Config` struct), `Effective*` fallbacks, `Load` / `Save` / `Remove` |
| `internal/config/config_test.go` | Round-trip + tunable-fallback tests |
| `internal/resources/resources.go` | `//go:embed wrapper.sh` declaration |
| `internal/resources/wrapper.sh` | The wrapper shipped to every home (invoked by the backend job and by `aos run`) |
| `scripts/install.sh` | End-user build + install (`go build -trimpath -ldflags="-s -w"` → `~/.local/bin/aos` by default) |

## Data Model

### `~/.config/aos/config.toml`

```toml
aos_home = "/home/you/.aos"
runs_hard_cap = 2000
tick_interval = "1h"
```

- `runs_hard_cap` — falls back to `DefaultRunsHardCap` (2000) when ≤ 0; consumed via `(*Config).EffectiveRunsHardCap`.
- `tick_interval` — Go duration string. `EffectiveTickInterval` parses it; values shorter than 1 minute are rejected (a non-nil error is returned alongside `DefaultTickInterval` so the caller can log it). Default is `"1h"` — the platform backends make up missed wakes themselves, so the tick only needs to run hourly for the work it still owns (miss detection, stale-running sweep, drift probe).
- Unknown keys in the file are silently dropped (`DisallowUnknownFields` is *not* set) so upgrading from a cron-era config with `catchup_enabled = true` doesn't error out — the field is just ignored.

### `<aos_home>/` layout

```
<aos_home>/
├── agents/                 # user-authored scripts + sidecars (preserved on uninstall)
├── runs/                   # wrapper-written <run-id>.json + <run-id>.out (preserved on uninstall)
├── wrapper.sh              # embedded copy, mode 0755
└── tick.log                # appended per tick (see scheduler feature; removed on uninstall)
```

Note: the platform scheduler entries (plists on macOS, `.timer`+`.service` pairs on Linux) live **outside** `aos_home`, in OS-owned directories:

- macOS: `~/Library/LaunchAgents/com.agenticos.*.plist`
- Linux: `~/.config/systemd/user/agentic-os-*.{service,timer}`

`backend.Remove()` is what cleans these up on uninstall.

## Noteworthy Behavior

- **Wrapper is the source of truth on disk.** `ensureHome` compares the on-disk bytes against `resources.WrapperSh` and only rewrites when they drift. `state` is `wrote` or `same`. After writing, the mode is force-`Chmod`ed to 0755 — `os.WriteFile` honors the user's umask so the file may otherwise land 0644 and break the backend execs.
- **Relocation tolerates cross-device moves.** `os.Rename` is attempted first; on `LinkError` with `EXDEV`, `copyTree` walks the source and `os.RemoveAll`s the original. Symlinks are preserved (`os.Symlink`), not dereferenced. A non-empty target directory is rejected with a "resolve manually" error — the verb refuses to merge into a populated path.
- **Init merges, doesn't replace.** `mergeInitConfig` copies the existing config and only overrides `AosHome`; user-set values for `RunsHardCap` and `TickInterval` survive a re-`init`. Defaults are filled in only when the existing field was zero/empty.
- **Linger prompt is interactive-only.** `maybePromptLinger` skips entirely when `JSONOutput()` is true, when `LingerState == HealthOK`, or when the state field is empty (non-Linux). On a desktop session (`XDG_SESSION_TYPE` set to anything other than `""` or `"tty"`), it prints a one-line info note that linger-off is fine in that environment. On a headless session without a TTY (e.g. running from a deploy script), it prints a warning with the manual command but doesn't try to prompt.
- **`aos home` prints raw path on stdout.** No lipgloss styling, no banner. Existing shell patterns like `cd "$(aos home)"` predate the styled CLI and the verb deliberately stays scriptable.
- **Uninstall is best-effort.** Each surface (wrapper, tick.log, backend, config file) is removed independently; failures are reported on stderr but don't short-circuit the rest. A user with a half-broken install can still get most of the cleanup done in one call. The summary's `backend` field reports `removed` on success or `skipped:<reason>` (e.g. `skipped:no-config`, `skipped:<select-error>`) when the backend wasn't reachable.
- **In-process refresh, not subprocess.** `init.go` calls `runRefresh()` directly. This shares the process's resolved `os.Executable()` so the tick job's argv bakes in the same binary path the user just installed — without that, a refresh shelled out via `PATH` would risk picking up a stale `aos` from elsewhere.

## Dependencies

- `internal/resources` — embedded `wrapper.sh`.
- `internal/scheduler/backend` — `Backend.Remove` invoked by uninstall; `Backend.Sync` invoked transitively via the scheduler feature's `Refresh`.
- `internal/runtime` — `os.Executable()` resolution baked into the tick job's argv during the embedded refresh.
- `github.com/pelletier/go-toml/v2` — config parser.
- `charm.land/lipgloss/v2`, `github.com/charmbracelet/x/term` — styled output and TTY detection for the linger prompt.
