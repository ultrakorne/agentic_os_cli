#!/usr/bin/env bash
# agentic_os run wrapper
# usage: wrapper.sh <data-dir> <schedule-id|''> <agent-id> <agent-script>
#
# captures start/end/exit/output of <agent-script> and writes:
#   <data-dir>/runs/<run-id>.json   meta record
#   <data-dir>/runs/<run-id>.out    captured stdout+stderr
#
# trigger defaults to 'schedule'; manual runs from the UI set
# AGENTIC_OS_TRIGGER=manual in env.

set -uo pipefail

if [ $# -lt 4 ]; then
  echo "wrapper.sh: expected 4 args, got $#" >&2
  exit 64
fi

DATA_DIR="$1"
SCHED_ID="$2"
AGENT_ID="$3"
SCRIPT="$4"
# optional 5th arg: caller-provided run id (manual runs from the engine pass
# this so the spawn-time stub matches the on-disk record).
EXPLICIT_RUN_ID="${5:-}"
TRIGGER="${AGENTIC_OS_TRIGGER:-schedule}"

# cron's PATH is minimal; give scripts a fighting chance
export PATH="/usr/local/bin:/opt/homebrew/bin:/usr/bin:/bin:${PATH:-}"

if [ -n "$EXPLICIT_RUN_ID" ]; then
  RUN_ID="$EXPLICIT_RUN_ID"
else
  RUN_ID="$(date +%s)-$$-${RANDOM}${RANDOM}"
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
  # args: status ended exitCode
  local status="$1" ended="$2" exit_code="$3"
  python3 - "$META.tmp" "$RUN_ID" "$AGENT_ID" "$SCHED_ID" "$START" "$TRIGGER" "$status" "$ended" "$exit_code" "$RUN_ID.out" <<'PY'
import json, sys
p, rid, jid, sid, start, trig, status, ended, ec, out_name = sys.argv[1:]
data = {
  "id": rid,
  "jobId": jid,
  "scheduleId": sid or None,
  "trigger": trig,
  "startedAt": start,
  "endedAt": ended or None,
  "status": status,
  "exitCode": int(ec) if ec else None,
  "output": "",
  "error": None,
  "outputPath": out_name,
}
with open(p, "w") as f:
  json.dump(data, f)
PY
  mv "$META.tmp" "$META"
}

START="$(iso_now)"
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
else
  STATUS="error"
fi

write_meta "$STATUS" "$END" "$EC"
