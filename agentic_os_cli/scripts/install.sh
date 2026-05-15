#!/usr/bin/env bash
# Build aos (prod flags) and install to ~/.local/bin.
#
# The crontab managed block bakes in the absolute path of the binary, so
# cron's minimal PATH does not need to include the install dir. The only
# thing PATH affects is your interactive shell — if ~/.local/bin is not
# on it, this script prints a hint.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

INSTALL_DIR="${AOS_INSTALL_DIR:-$HOME/.local/bin}"
INSTALL_PATH="$INSTALL_DIR/aos"

cd "$REPO_ROOT"

if ! command -v go >/dev/null 2>&1; then
  echo "install: go toolchain not found on PATH" >&2
  exit 1
fi

mkdir -p "$INSTALL_DIR"

echo "build: go build -trimpath -ldflags=\"-s -w\" -> $INSTALL_PATH"
GOFLAGS="${GOFLAGS:-}" go build -trimpath -ldflags="-s -w" -o "$INSTALL_PATH" ./cmd/aos
chmod 0755 "$INSTALL_PATH"

echo "install: $INSTALL_PATH ($($INSTALL_PATH --help >/dev/null 2>&1 && echo ok || echo 'warn: binary not runnable'))"

case ":${PATH:-}:" in
  *":$INSTALL_DIR:"*) ;;
  *)
    echo
    echo "note: $INSTALL_DIR is not on your interactive PATH."
    echo "      add to ~/.bashrc or ~/.zshrc:"
    echo "          export PATH=\"$INSTALL_DIR:\$PATH\""
    echo "      (cron is unaffected — the managed block uses the absolute path.)"
    ;;
esac

if [ -f "$HOME/.config/aos/config.toml" ]; then
  echo
  echo "next: run 'aos refresh' to rebuild the crontab with the new binary path."
fi
