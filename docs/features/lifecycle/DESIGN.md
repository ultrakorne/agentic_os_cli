# lifecycle — Design

## Overview

Lifecycle covers the three verbs that decide where the aos home lives, materialize the on-disk layout, and tear it down. The contract is "one place to look on disk, one place to look in config, never duplicate truth": the user names a path with `aos init`, that path goes into `~/.config/aos/config.toml`, every other verb reads it back via `aos home` semantics.

## Components

### `aos init <path>`

Bootstraps everything. Creates the chosen home directory, writes `wrapper.sh` from the binary's embedded copy, seeds empty `agents/` and `runs/` subdirectories, records `<path>` (plus defaults for `runs_hard_cap`, `catchup_enabled`, `tick_interval`) in `~/.config/aos/config.toml`, then runs an in-process `aos refresh` so the managed crontab block is installed in the same call. Defaults are written **explicitly** to the config file so opening it reveals the available knobs without reading docs.

Re-running `init` against a new path **relocates** the existing home: same-device renames are preferred; cross-device fallback is copy+remove. The previous home's `agents/` and `runs/` move with it. User-set tunables in the existing config (`runs_hard_cap`, `catchup_enabled`, `tick_interval`) are preserved; only `aos_home` is rewritten.

### `aos home`

The "where is everything?" probe. Prints the absolute `aos_home` path on stdout — plain (no styling) so `$(aos home)/runs` keeps working — or `{"home": "<path>"}` under `--json`. Exits non-zero if `aos init` hasn't run yet.

### `aos uninstall`

The reverse of `init`. Removes `wrapper.sh`, the managed crontab block, and `~/.config/aos/config.toml`. **User data is preserved**: `agents/` and `runs/` are removed *only* if empty. Anything non-empty surfaces in the `kept` field so the operator can decide whether to delete it manually. The verb is safe to run when aos was never initialized (everything reports `absent` / `unchanged`).

## User Flows

### First-time install

1. User builds the binary (`scripts/install.sh` → `~/.local/bin/aos`).
2. User runs `aos init ~/.aos`.
3. The home directory is created with `agents/` and `runs/`; `wrapper.sh` is written.
4. The config file is written with every default populated explicitly.
5. An in-process refresh runs and installs the managed crontab block.
6. The verb prints `mode=fresh` plus the refresh summary.

### Relocating the home

1. User runs `aos init ~/new-aos-path`.
2. Init detects the existing config pointing elsewhere, attempts `os.Rename` of the old home onto the new path, and falls back to a recursive copy if `Rename` returns `EXDEV` (cross-device).
3. The config is updated to point at the new path; the same in-process refresh follows.
4. The verb prints `mode=moved` (rename succeeded) or `mode=repointed` (no old home was present, config was just stale).

### Uninstall

1. User runs `aos uninstall`.
2. `wrapper.sh` is removed; `agents/` and `runs/` are removed if empty (otherwise listed under `kept`).
3. The managed crontab block is stripped; the rest of the user's crontab is untouched.
4. `~/.config/aos/config.toml` is removed; the `~/.config/aos/` directory is removed if empty.
5. A summary prints showing `removed | absent | unchanged | skipped:<reason>` per surface.

## Design Decisions

- **Init is idempotent.** Running `aos init <same-path>` twice produces `mode=same` and a no-op refresh — the wrapper is only rewritten if its on-disk bytes drift from the embedded copy.
- **Defaults live in the file, not in the docs.** Writing `runs_hard_cap = 2000`, `catchup_enabled = true`, `tick_interval = "10m"` explicitly makes the knobs discoverable to anyone opening the config.
- **Relocation is single-call.** A user changing where they want the home should be able to do it with one `aos init <new-path>` — no separate "move" verb, no manual config edit, no two-step dance.
- **Uninstall never deletes user data.** Scripts under `agents/` and run records under `runs/` are user-authored or wrapper-written; the verb refuses to clean them up silently. The `kept` field tells the operator what's left.
- **Cron is reconciled in-process, not by shelling out to `aos refresh`.** Init calls `RunRefresh()` directly so the user sees one combined report, not two separate executions with split exit codes.
