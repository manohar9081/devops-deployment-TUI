#!/bin/bash

set -euo pipefail

LOCAL_BIN="$HOME/.local/bin"
SCRIPT_DIR="$HOME/devops-deployment/scripts"
SCRIPT_TMPDIR="$HOME/devops-deployment/tmp"
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

mkdir -p "$SCRIPT_TMPDIR" "$SCRIPT_DIR"

MIRROR="https://mirror.openshift.com/pub/openshift-v4/clients/oc"

# The mirror's "latest" stream publishes linux (x86_64) and macosx tarballs.
# There is no ARM build under latest — ARM users need a versioned path.
case "$OS" in
  linux)
    case "$ARCH" in
      x86_64)              OC_URL="$MIRROR/latest/linux/oc.tar.gz" ;;
      aarch64|arm64)       OC_URL="$MIRROR/latest/linux-arm64/oc.tar.gz" ;;
      *) echo "Unsupported architecture for oc: $ARCH"; exit 1 ;;
    esac
    ;;
  darwin)
    OC_URL="$MIRROR/latest/macosx/oc.tar.gz"
    ;;
  *)
    echo "Unsupported OS: this installer handles Linux/macOS (got '$OS')."
    exit 1
    ;;
esac

# Fail fast with guidance when the mirror has no build for this arch.
if ! curl -fsIL -o /dev/null --max-time 20 "$OC_URL"; then
  echo "No oc tarball found at: $OC_URL"
  echo "ARM note: the mirror's 'latest' stream has no ARM build. Download a"
  echo "versioned tarball instead, e.g.:"
  echo "  https://mirror.openshift.com/pub/openshift-v4/clients/oc/<version>/linux/arm64/oc.tar.gz"
  exit 1
fi

echo "Downloading oc client from: $OC_URL"
curl --fail -L -o "$SCRIPT_TMPDIR/oc.tar.gz" "$OC_URL"

echo "Extracting oc..."
EXTRACT_DIR=$(mktemp -d)
tar -xzf "$SCRIPT_TMPDIR/oc.tar.gz" -C "$EXTRACT_DIR"

cp "$EXTRACT_DIR/oc" "$LOCAL_BIN/oc"
chmod +x "$LOCAL_BIN/oc"

rm -rf "$EXTRACT_DIR"
rm -rf "${SCRIPT_TMPDIR:?}/"*

echo "oc is now installed..."
"$LOCAL_BIN/oc" version --client 2>/dev/null || true
