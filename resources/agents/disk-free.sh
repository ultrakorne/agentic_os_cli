#!/usr/bin/env bash
# disk-free — sample agent. Reports disk usage of the root filesystem.
set -euo pipefail
df -h /
