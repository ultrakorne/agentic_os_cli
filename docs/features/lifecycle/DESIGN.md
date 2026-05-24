# lifecycle — Design

## Overview

Lifecycle covers the three verbs that decide where the aos home lives, materialize the on-disk layout, and tear it down. The contract is "one place to look on disk, one place to look in config, never duplicate truth": the user names a path with `aos init`, that path goes into `~/.config/aos/config.toml`, every other verb reads it back via `aos home` semantics.

## Components

### `aos init <path>`

Bootstraps everything. Creates the chosen home directory, writes `wrapper.sh` from the binary's embedded copy, seeds empty `agents/` and `runs/` subdirectories, records `<path>` (plus defaults for `runs_hard_cap` and `tick_interval`) in `~/.config/aos/config.toml`, then runs an in-process `aos refresh` so the platform-native scheduler entries (launchd LaunchAgents on macOS, systemd-user `.timer` + `.service` units on Linux) get installed in the same call. Defaults are written **explicitly** to the config file so opening it reveals the available knobs without reading docs.

On Linux, after refresh, init inspects `RefreshOutcome.LingerState`. If linger is disabled on a headless session (`XDG_SESSION_TYPE` empty or `"tty"`) and the operator is at a TTY, init prints a warning and offers to run `sudo loginctl enable-linger $USER` interactively. Under `--json` the prompt is skipped (the state is still in the outcome).

Re-running `init` against a new path **relocates** the existing home: same-device renames are preferred; cross-device fallback is copy+remove. The previous home's `agents/` and `runs/` move with it. User-set tunables in the existing config (`runs_hard_cap`, `tick_interval`) are preserved; only `aos_home` is rewritten.

### `aos home`

The "where is everything?" probe. Prints the absolute `aos_home` path on stdout — plain (no styling) so `$(aos home)/runs` keeps working — or `{"home": "<path>"}` under `--json`. Exits non-zero if `aos init` hasn't run yet.

### `aos uninstall`

The reverse of `init`. Removes `wrapper.sh`, the platform scheduler entries (via `backend.Remove()` — boots-out every LaunchAgent in the `com.agenticos.*` namespace, or disables every `.timer` and unlinks both halves of the `agentic-os-*` pairs), the `tick.log`, and `~/.config/aos/config.toml`. **User data is preserved**: `agents/` and `runs/` are left in place. The verb is safe to run when aos was never initialized (the backend step reports `skipped:no-config`).

## User Flows

### First-time install

1. User builds the binary (`scripts/install.sh` → `~/.local/bin/aos`).
2. User runs `aos init ~/.aos`.
3. The home directory is created with `agents/` and `runs/`; `wrapper.sh` is written.
4. The config file is written with every default populated explicitly.
5. An in-process refresh runs and installs the platform scheduler entries via `backend.Sync` (writes the per-agent plists / unit pairs and the tick entry, then bootstraps / `enable --now`s each one).
6. On Linux, if linger is disabled on a headless host, init offers to run `sudo loginctl enable-linger $USER`.
7. The verb prints `mode=fresh` plus the refresh summary.

### Relocating the home

1. User runs `aos init ~/new-aos-path`.
2. Init detects the existing config pointing elsewhere, attempts `os.Rename` of the old home onto the new path, and falls back to a recursive copy if `Rename` returns `EXDEV` (cross-device).
3. The config is updated to point at the new path; the same in-process refresh follows. Because every backend entry's argv references the absolute wrapper / aos_home / script paths, the refresh rewrites every plist or unit to point at the new location.
4. The verb prints `mode=moved` (rename succeeded) or `mode=repointed` (no old home was present, config was just stale).

### Uninstall

1. User runs `aos uninstall`.
2. `wrapper.sh` and `tick.log` are removed.
3. `backend.Remove()` boots-out every LaunchAgent in the namespace (macOS) or `disable --now`s every `.timer` and unlinks both halves of each pair (Linux), then `daemon-reload`s.
4. `~/.config/aos/config.toml` is removed; the `~/.config/aos/` directory is removed if empty.
5. `agents/` and `runs/` are left in place — they hold user-authored scripts and run history.
6. A summary prints showing `removed | absent | skipped:<reason>` per surface.

## Design Decisions

- **Init is idempotent.** Running `aos init <same-path>` twice produces `mode=same` and a no-op refresh — the wrapper is only rewritten if its on-disk bytes drift from the embedded copy, and the backend reports `unchanged=N` when nothing in the namespace needs touching.
- **Defaults live in the file, not in the docs.** Writing `runs_hard_cap = 2000`, `tick_interval = "1h"` explicitly makes the knobs discoverable to anyone opening the config. The `catchup_enabled` field that used to live here is gone — native makeup handles the role.
- **Relocation is single-call.** A user changing where they want the home should be able to do it with one `aos init <new-path>` — no separate "move" verb, no manual config edit, no two-step dance.
- **Uninstall never deletes user data.** Scripts under `agents/` and run records under `runs/` are user-authored or wrapper-written; the verb refuses to clean them up silently. The operator removes the directories explicitly if they want them gone.
- **Backend reconciliation is in-process, not by shelling out to `aos refresh`.** Init calls `runRefresh()` directly so the user sees one combined report, not two separate executions with split exit codes. This also shares the resolved `os.Executable()` so the tick job's argv bakes in the same binary path the user just installed.
- **Linger is prompted-for, not enforced.** Init can't enable linger non-interactively (it needs sudo and a TTY); on headless hosts it surfaces the warning and offers the command, but exits cleanly either way. Refresh re-warns through `LingerState` so a user who said "no" the first time sees it again later.
