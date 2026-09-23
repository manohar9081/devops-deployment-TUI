#!/bin/bash

set -euo pipefail

# SCRIPT_DIR/SCRIPT_TMPDIR come from the Go menu environment when set (the
# materialized embedded assets); these defaults self-locate the checkout —
# the script's own dir and, two levels up, the repo tmp/ — so direct
# standalone runs from assets/scripts/ work too.
SCRIPT_DIR="${SCRIPT_DIR:-$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" && pwd -P)}"
SCRIPT_TMPDIR="${SCRIPT_TMPDIR:-$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")/../.." && pwd -P)/tmp}"
LOCAL_BIN="$HOME/.local/bin"

mkdir -p "$SCRIPT_TMPDIR" "$SCRIPT_DIR"

if [[ "$(uname -s)" != "Linux" ]]; then
  echo "This installer downloads a Linux AppImage and only runs on Linux."
  exit 1
fi

ARCH="${1:-x86_64}"
case "$ARCH" in
  arm64) ARCH="aarch64" ;;
esac

# Resolve the newest Helium AppImage asset that actually exists in the
# release repo (newest release first), so the URL always points at a
# downloadable asset even when asset names or tags vary.
echo "Looking up the newest Helium AppImage (arch: $ARCH)..."
DOWNLOAD_URL=$(curl -fsSL "https://api.github.com/repos/imputnet/helium-linux/releases?per_page=10" \
  | jq -r --arg arch "$ARCH" '
      .[].assets[]
      | select(.name | test("^helium-.*-" + $arch + "\\.AppImage$"))
      | .browser_download_url' \
  | head -n 1)

if [[ -z "$DOWNLOAD_URL" ]]; then
  echo "No Helium AppImage found in the releases (arch: $ARCH)."
  exit 1
fi
echo "Downloading: $DOWNLOAD_URL"

# Check and gracefully stop a running Helium process before replacing the binary.
# Try SIGTERM first (lets Helium save session/tabs), fall back to SIGKILL.
pid=$(pgrep -x helium || true)
if [[ -n "$pid" ]]; then
  echo "Helium is running (pid: $pid). Asking it to quit (SIGTERM)..."
  kill "$pid" 2>/dev/null || true
  for _ in {1..10}; do
    if ! pgrep -x helium >/dev/null 2>&1; then
      break
    fi
    sleep 0.5
  done
  if pgrep -x helium >/dev/null 2>&1; then
    echo "Helium did not exit gracefully. Forcing (SIGKILL)..."
    pkill -9 -x helium || true
  fi
  echo "Helium stopped."
else
  echo "Helium is not running."
fi

# Download Helium Browser as AppImage
wget -O "$SCRIPT_TMPDIR/helium.AppImage" "$DOWNLOAD_URL"
cp -p "$SCRIPT_TMPDIR/helium.AppImage" "$LOCAL_BIN/"
chmod +x "$LOCAL_BIN/helium.AppImage"

rm -rf "${SCRIPT_TMPDIR:?}/"*
echo "Helium Browser is now installed..."
