#!/usr/bin/env bash
# agentic_os run wrapper
# usage: wrapper.sh   (no positional args; all input comes from env vars)
#
# required env:
#   AGENTIC_OS_DATA_DIR        aos_home (runs/ lives here)
#   AGENTIC_OS_AGENT_ID        agent identifier (filename stem)
#   AGENTIC_OS_AGENT_SCRIPT    absolute path to the agent script
# optional env:
#   AGENTIC_OS_RUN_ID          caller-provided run id; otherwise wrapper mints one
#   AGENTIC_OS_TRIGGER         schedule | manual; defaults to schedule
#
# captures start/end/exit/output of the agent script and writes:
#   $AGENTIC_OS_DATA_DIR/runs/<run-id>.json   meta record
#   $AGENTIC_OS_DATA_DIR/runs/<run-id>.out    captured stdout+stderr
#
# TIMESTAMP FORMAT: iso_now below MUST emit the same shape the Go side
# normalizes on — millisecond-precision UTC, e.g. "2026-05-16T13:09:37.072Z".
# See scheduler.RunTimestampFormat / FormatRunTimestamp. Records that diverge
# break same-second lexicographic sorting; tests live in
# internal/scheduler/wrapper_format_test.go.

set -uo pipefail

# Every meta write goes through python3; without it the script would corrupt
# records silently.
if ! command -v python3 >/dev/null 2>&1; then
  echo "wrapper.sh: python3 not found on PATH; cannot write run records" >&2
  exit 127
fi

DATA_DIR="${AGENTIC_OS_DATA_DIR:-}"
AGENT_ID="${AGENTIC_OS_AGENT_ID:-}"
SCRIPT="${AGENTIC_OS_AGENT_SCRIPT:-}"
if [ -z "$DATA_DIR" ] || [ -z "$AGENT_ID" ] || [ -z "$SCRIPT" ]; then
  echo "wrapper.sh: missing required env (AGENTIC_OS_DATA_DIR, AGENTIC_OS_AGENT_ID, AGENTIC_OS_AGENT_SCRIPT)" >&2
  exit 64
fi
EXPLICIT_RUN_ID="${AGENTIC_OS_RUN_ID:-}"
TRIGGER="${AGENTIC_OS_TRIGGER:-schedule}"

# cron's PATH is minimal; cover the common per-user bin dirs so installed
# tools like claude (which lands in ~/.local/bin) are findable. Agent scripts
# needing anything past this are responsible for extending PATH themselves.
export PATH="$HOME/.local/bin:$HOME/bin:/usr/local/bin:/opt/homebrew/bin:/usr/bin:/bin:${PATH:-}"

if [ -n "$EXPLICIT_RUN_ID" ]; then
  RUN_ID="$EXPLICIT_RUN_ID"
else
  # mirror engine-side newRunID: <unix-millis>-<rand4>. GNU date supports
  # %3N; BSD/macOS date leaves the specifier literal, in which case the
  # result contains non-digits — fall back to seconds*1000.
  MS="$(date +%s%3N)"
  case "$MS" in
    *[!0-9]*) MS="$(date +%s)000" ;;
  esac
  RUN_ID="${MS}-$(printf '%04x' "$RANDOM")"
fi
RUNS_DIR="$DATA_DIR/runs"
mkdir -p "$RUNS_DIR"

META="$RUNS_DIR/$RUN_ID.json"
OUT="$RUNS_DIR/$RUN_ID.out"

# Expose context to the child script so user agents can write portable
# paths like "$AGENTIC_OS_DATA_DIR/workspaces/foo" instead of hard-coding
# the platform-specific userData root.
export AGENTIC_OS_DATA_DIR="$DATA_DIR"
export AGENTIC_OS_AGENT_ID="$AGENT_ID"
export AGENTIC_OS_AGENT_SCRIPT="$SCRIPT"
export AGENTIC_OS_RUN_ID="$RUN_ID"
export AGENTIC_OS_TRIGGER="$TRIGGER"

iso_now() {
  # millisecond ISO-8601 UTC; macOS `date` lacks %3N so use python
  python3 -c 'import datetime,sys; sys.stdout.write(datetime.datetime.now(datetime.timezone.utc).isoformat(timespec="milliseconds").replace("+00:00","Z"))'
}

write_meta() {
  # args: status ended exitCode [error]
  local status="$1" ended="$2" exit_code="$3" err_msg="${4:-}"
  python3 - "$META.tmp" "$RUN_ID" "$AGENT_ID" "$START" "$TRIGGER" "$status" "$ended" "$exit_code" "$RUN_ID.out" "$err_msg" <<'PY'
import json, sys
p, rid, jid, start, trig, status, ended, ec, out_name = sys.argv[1:10]
err = sys.argv[10] if len(sys.argv) > 10 and sys.argv[10] else None
data = {
  "id": rid,
  "agentId": jid,
  "trigger": trig,
  "startedAt": start,
  "endedAt": ended or None,
  "status": status,
  "exitCode": int(ec) if ec else None,
  "output": "",
  "error": err,
  "outputPath": out_name,
}
with open(p, "w") as f:
  json.dump(data, f)
PY
  mv "$META.tmp" "$META"
}

START="$(iso_now)"

# SIGTERM/SIGINT: launchd/systemd send SIGTERM on reload. Set a flag and let
# the foreground script exit naturally — a SIGTERM-aware script that flushes
# state and exits 0 should still be recorded as success.
INTERRUPTED=0
trap 'INTERRUPTED=1' SIGTERM SIGINT

write_meta running "" ""

# On macOS, scripts that arrived via download/scp/airdrop carry a
# `com.apple.quarantine` xattr and Gatekeeper blocks the interpreter with
# "bad interpreter: Operation not permitted". Strip it before exec; on
# Linux there's no xattr binary and the redirect makes it a no-op.
if [ "$(uname)" = "Darwin" ] && command -v xattr >/dev/null 2>&1; then
  xattr -d com.apple.quarantine "$SCRIPT" 2>/dev/null || true
fi

# exec the script directly so its shebang picks the interpreter
# (the scanner enforces +x, so this works for any language).
"$SCRIPT" >"$OUT" 2>&1
EC=$?

END="$(iso_now)"
if [ "$EC" -eq 0 ]; then
  STATUS="success"
  write_meta "$STATUS" "$END" "$EC"
elif [ "$INTERRUPTED" = "1" ]; then
  STATUS="error"
  write_meta "$STATUS" "$END" "$EC" "interrupted by reload"
else
  STATUS="error"
  # If a failing script produced no output, write a hint to the log so the
  # dashboard shows something actionable instead of an empty error row. Most
  # often this is a script that captured everything via $(...) and exited
  # before replaying it (claude --print errors land on stdout, not stderr).
  if [ ! -s "$OUT" ]; then
    printf '[wrapper] script exited %d with no output.\nLikely cause: stdout was captured via $(...) or `cmd` and the script exited before replaying it. Capture stderr too (`$(cmd 2>&1)`) and echo it on failure.\n' "$EC" >"$OUT"
  fi
  write_meta "$STATUS" "$END" "$EC"
fi
