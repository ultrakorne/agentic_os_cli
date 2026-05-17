# lifecycle — Technical

## Architecture

Lifecycle is split across two layers. The verb layer in `cmd/aos/` parses flags, decides modes (`fresh|same|moved|repointed`), and emits human or JSON output; the persistence layer in `internal/config/` owns the TOML file and the `~/.config/aos/` directory. `wrapper.sh` is embedded into the binary at build time via `//go:embed` so a single binary install carries everything needed to bootstrap a home. Cron reconciliation is *not* implemented here — both `init` and `uninstall` delegate to `internal/crontab` via the scheduler feature.

## Source Files

| File | Role |
|------|------|
| `cmd/aos/init.go` | `aos init` verb: argument expansion, relocation, embedded-wrapper write, config merge, in-process refresh |
| `cmd/aos/init_test.go` | Init tests: same-path no-op, fresh install, relocation (in-tree + EXDEV simulation), config merge |
| `cmd/aos/home.go` | `aos home` verb: read config, print path or `{"home": "<path>"}` |
| `cmd/aos/uninstall.go` | `aos uninstall` verb: orchestrates wrapper removal, empty-only data dir removal, managed-block strip, config removal |
| `internal/config/config.go` | TOML schema (`Config` struct), `Effective*` fallbacks, `Load` / `Save` / `Remove` |
| `internal/config/config_test.go` | Round-trip + tunable-fallback tests |
| `internal/resources/resources.go` | `//go:embed wrapper.sh` declaration |
| `internal/resources/wrapper.sh` | The wrapper shipped to every home (executed by cron and by `aos run`) |
| `scripts/install.sh` | End-user build + install (`go build -trimpath -ldflags="-s -w"` → `~/.local/bin/aos` by default) |

## Data Model

### `~/.config/aos/config.toml`

```toml
aos_home = "/home/you/.aos"
runs_hard_cap = 2000
catchup_enabled = true
tick_interval = "10m"
```

- `runs_hard_cap` — falls back to `DefaultRunsHardCap` (2000) when ≤ 0; consumed via `(*Config).EffectiveRunsHardCap`.
- `catchup_enabled` — stored as `*bool` so the absent-vs-explicit-false distinction is preserved on the wire; defaults to true.
- `tick_interval` — Go duration string. `EffectiveTickCronExpr` parses it into a crontab(5) expression: whole minutes in `[1, 59]` → `*/N * * * *`, whole hours in `[1, 23]` → `0 */H * * *`. Anything else falls back to `"10m"` with a non-nil error returned alongside so the caller can log it.

### `<aos_home>/` layout

```
<aos_home>/
├── agents/                 # user-authored scripts + sidecars (preserved on uninstall)
├── runs/                   # wrapper-written <run-id>.json + <run-id>.out (preserved on uninstall)
├── wrapper.sh              # embedded copy, mode 0755
├── tick.log                # appended per tick (see scheduler feature)
└── .crontab.lock           # process-local lock used by refresh
```

## Noteworthy Behavior

- **Wrapper is the source of truth on disk.** `ensureHome` compares the on-disk bytes against `resources.WrapperSh` and only rewrites when they drift. `state` is `wrote` or `same`. After writing, the mode is force-`Chmod`ed to 0755 — `os.WriteFile` honors the user's umask so the file may otherwise land 0644 and break cron.
- **Relocation tolerates cross-device moves.** `os.Rename` is attempted first; on `LinkError` with `EXDEV`, `copyTree` walks the source and `os.RemoveAll`s the original. Symlinks are preserved (`os.Symlink`), not dereferenced. A non-empty target directory is rejected with a "resolve manually" error — the verb refuses to merge into a populated path.
- **Init merges, doesn't replace.** `mergeInitConfig` copies the existing config and only overrides `AosHome`; user-set values for `RunsHardCap`, `CatchupEnabled`, and `TickInterval` survive a re-`init`. Defaults are filled in **only** when the existing field was zero/empty so a user who deliberately set `catchup_enabled = false` won't have it flipped back on by a future `init`.
- **`aos home` prints raw path on stdout.** No lipgloss styling, no banner. Existing shell patterns like `cd "$(aos home)"` predate the styled CLI and the verb deliberately stays scriptable.
- **Uninstall is best-effort and order-tolerant.** Each surface (wrapper, agents/, runs/, aos_home, crontab block, config file) is removed independently; failures are reported on stderr but don't short-circuit the rest. A user with a half-broken install can still get most of the cleanup done in one call.
- **The `kept` field is populated by `os.Remove` failures.** `os.Remove` only succeeds on an empty directory, so a `kept` entry literally means "this directory had user data in it."
- **In-process refresh, not subprocess.** `init.go` calls `RunRefresh()` directly. This shares the process's resolved `os.Executable()` so the managed cron line bakes in the same binary path the user just installed — without that, a refresh shelled out via `PATH` would risk picking up a stale `aos` from elsewhere.

## Dependencies

- `internal/resources` — embedded `wrapper.sh`.
- `internal/crontab` — managed-block sync/remove (via the scheduler feature's `RunRefresh`).
- `internal/runtime` — `os.Executable()` resolution baked into the cron line during the embedded refresh.
- `github.com/pelletier/go-toml/v2` — config parser.
