#!/usr/bin/env bash
# ping — minimal sample agent. Sleeps briefly, then echoes a timestamp.
# Replace or delete; agentic_os will not regenerate this file.

set -euo pipefail
sleep 2
echo "ping at $(date -u +%FT%TZ)"
