#!/bin/bash

set -euo pipefail

LOCAL_BIN="$HOME/.local/bin"
# SCRIPT_DIR/SCRIPT_TMPDIR come from the Go menu environment when set (the
# materialized embedded assets); these defaults self-locate the checkout —
# the script's own dir and, two levels up, the repo tmp/ — so direct
# standalone runs from assets/scripts/ work too.
SCRIPT_DIR="${SCRIPT_DIR:-$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" && pwd -P)}"
SCRIPT_TMPDIR="${SCRIPT_TMPDIR:-$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")/../.." && pwd -P)/tmp}"
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

mkdir -p "$SCRIPT_TMPDIR" "$SCRIPT_DIR"

if [[ "$ARCH" == "x86_64" ]]; then
  ARCH="amd64"
elif [[ "$ARCH" == "aarch64" || "$ARCH" == "arm64" ]]; then
  ARCH="arm64"
else
  echo "Unsupported architecture: $ARCH"
  exit 1
fi

get_k9s_latest_version() {
  K9S_VERSION=$(curl -fsSL https://api.github.com/repos/derailed/k9s/releases/latest | jq -r .tag_name | sed 's/^v//')
}

echo "Enter the k9s version you want to download (press Enter for the latest version):"
read -r USER_VERSION

if [[ -z "$USER_VERSION" ]]; then
  echo "No version specified, fetching the latest stable version..."
  get_k9s_latest_version
else
  K9S_VERSION="$USER_VERSION"
  echo "Downloading k9s version $K9S_VERSION..."
fi
echo "Latest k9s version: $K9S_VERSION"

DOWNLOAD_URL="https://github.com/derailed/k9s/releases/download/v$K9S_VERSION/k9s_${OS}_${ARCH}.tar.gz"
echo "Downloading k9s from: $DOWNLOAD_URL"
curl --fail -L -o "$SCRIPT_TMPDIR/k9s-$K9S_VERSION.tar.gz" "$DOWNLOAD_URL"

# Extract to a /tmp dir (mktemp -d): on some VM/shared-folder filesystems tar's
# file creation fails with "Function not implemented"; /tmp (ext4/tmpfs) works.
echo "Untar $SCRIPT_TMPDIR/k9s-$K9S_VERSION.tar.gz"
EXTRACT_DIR="$(mktemp -d)"
tar -xzf "$SCRIPT_TMPDIR/k9s-$K9S_VERSION.tar.gz" -C "$EXTRACT_DIR"
chmod +x "$EXTRACT_DIR/k9s"

mv "$EXTRACT_DIR/k9s" "$LOCAL_BIN/"

rm -rf "$EXTRACT_DIR" "${SCRIPT_TMPDIR:?}/"*
echo "K9S is now installed..."
