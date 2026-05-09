#!/usr/bin/env bash
# ping — minimal sample agent. Echoes a timestamp; exits 0.
# Replace or delete; agentic_os will not regenerate this file.

set -euo pipefail
echo "ping at $(date -u +%FT%TZ)"
