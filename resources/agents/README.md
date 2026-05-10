# agents/

Each shell script in this directory (or a first-level subdirectory) is an
agent. The filename without extension is the agent's id.

## Sections from folders

The dashboard groups agents into sections. The section is determined by
the script's parent directory:

```
agents/
  ping.sh                       → id="ping",    section="Agents"
  Other/
    example.sh                  → id="example", section="Other"
```

`mv` files between folders to reorganize. IDs must be unique across the
whole tree — two scripts named `ping.sh` in different subfolders is a
collision; the dashboard keeps the first one it walks (top level wins
over subfolders, then subfolders alphabetically) and warns about the
duplicate in the main-process log.

Deeper nesting (`agents/Other/sub/foo.sh`) is ignored.

## Contract

- **Shell scripts only.** The dashboard discovers files with these
  extensions: `.sh`, `.bash`, `.zsh`, or no extension. To run something
  written in another language, wrap it in a one-line shell script (see
  "Non-shell agents" below).
- The script must be **executable** (`chmod +x`) and start with a valid
  shebang, e.g. `#!/usr/bin/env bash`.
- The script is invoked by `wrapper.sh`, which captures stdout+stderr and
  the exit code into `<userData>/data/runs/<run-id>.{json,out}`.
- Exit code 0 = success; anything else = error.
- `cron` runs scripts with a minimal environment. The wrapper sets
  `PATH=/usr/local/bin:/opt/homebrew/bin:/usr/bin:/bin:$PATH`. If your
  script needs more (Node, Python venv, Homebrew on Apple Silicon outside
  the default path, project-specific tools), set it inside the script.
- Network and credentials are your responsibility — the wrapper passes the
  user's environment through unchanged, but cron itself starts with very
  little of it.

## Wrapper environment

The wrapper exports a small set of env vars before invoking your script,
so you can write portable paths that work the same on Linux and macOS:

| Variable | Value |
|---|---|
| `AGENTIC_OS_DATA_DIR` | The data root (`<userData>/data`) |
| `AGENTIC_OS_AGENT_ID` | Id of the agent being run |
| `AGENTIC_OS_AGENT_SCRIPT` | Absolute path to the script being run |
| `AGENTIC_OS_RUN_ID` | Run id (matches the `runs/<id>.{json,out}` filenames) |
| `AGENTIC_OS_TRIGGER` | `schedule` or `manual` |

Use them with a fallback so the script is also runnable by hand:

```bash
DATA_DIR="${AGENTIC_OS_DATA_DIR:-$HOME/.config/agentic-os/data}"
WORKDIR="$DATA_DIR/workspaces/my_agent"
```

## Workspaces (optional)

If an agent needs a working directory — prompt files, state it reads/writes,
caches — drop a folder under `<userData>/data/workspaces/<name>/` and
reference it via `$AGENTIC_OS_DATA_DIR/workspaces/<name>`. Many agents
don't need one; skip it when you don't.

## Non-shell agents

To run a Python / Node / Ruby / etc. script, drop a thin shell wrapper here
that `exec`s the real program:

```bash
#!/usr/bin/env bash
# agents/example.sh
exec python3 "$HOME/scripts/example.py" "$@"
```

`exec` replaces the shell process with the interpreter so the wrapper still
sees the real exit code and captures the real output. The shim makes the
language explicit when you `cat agents/<id>` or read `crontab -l`.

## Optional metadata

Agent display metadata (title, description) and the schedule itself live
in `<userData>/data/agents.json`, not next to the script:

```json
{
  "agents": [
    {
      "id": "example",
      "title": "Example",
      "description": "What this agent does in one line.",
      "schedule": { "kind": "daily", "days": ["mon","tue","wed","thu","fri"], "hour": 9, "minute": 0 },
      "scheduledAt": "2026-05-09T08:30:00.000Z"
    }
  ]
}
```

Title and description are optional; missing values are derived from the
filename. Description is editable directly from the agent detail panel
(click any card). `scheduledAt` is set whenever the schedule changes and
is used to suppress retroactive missed-run reports for ticks that
predate the current schedule. Section comes from the script's parent
folder, not from this file.

## Adding an agent

1. Drop an executable script into `agents/` (top level) or
   `agents/<Section>/`.
2. Open the dashboard. The agent appears with a default title; create a
   schedule from the inline editor.
