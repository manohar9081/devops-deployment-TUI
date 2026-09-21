#!/bin/bash

set -euo pipefail

LOCAL_BIN="$HOME/.local/bin"
SCRIPT_DIR="$HOME/devops-deployment/scripts"
SCRIPT_TMPDIR="$HOME/devops-deployment/tmp"
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

mkdir -p "$SCRIPT_TMPDIR" "$SCRIPT_DIR"

# macOS: CLI via the official universal bundle; SSM plugin via brew cask.
if [[ "$OS" == "darwin" ]]; then
  echo "macOS detected: installing AWS CLI (universal) ..."
  curl --fail -L -o "$SCRIPT_TMPDIR/awscliv2.zip" \
    "https://awscli.amazonaws.com/awscli-exe-macos-universal.zip"
  unzip -q "$SCRIPT_TMPDIR/awscliv2.zip" -d "$SCRIPT_TMPDIR"
  "$SCRIPT_TMPDIR/aws/install" -i "$LOCAL_BIN/aws-cli" -b "$LOCAL_BIN" --update
  if ! command -v session-manager-plugin >/dev/null 2>&1; then
    if command -v brew >/dev/null 2>&1; then
      echo "Installing Session Manager plugin via Homebrew..."
      brew install --cask session-manager-plugin || \
        echo "WARN: cask failed — install the SSM plugin manually from AWS docs."
    else
      echo "Skipping SSM plugin: needs Homebrew (brew install --cask session-manager-plugin)."
    fi
  fi
  rm -rf "${SCRIPT_TMPDIR:?}/"*
  echo "AWS (macOS) is now installed..."
  exit 0
fi

if [[ "$OS" != "linux" ]]; then
  echo "Unsupported OS: this installer only handles Linux/macOS (got '$OS')."
  exit 1
fi

if [[ "$ARCH" == "x86_64" ]]; then
  ARCH="x86_64"
elif [[ "$ARCH" == "aarch64" || "$ARCH" == "arm64" ]]; then
  ARCH="aarch64"
else
  echo "Unsupported architecture: $ARCH"
  exit 1
fi

# Session Manager plugin (SSM). Use the native .deb on Debian/Ubuntu (no
# rpm2cpio/cpio needed), fall back to the .rpm on RPM-based distros.
if ! command -v session-manager-plugin >/dev/null 2>&1; then
  SSM_BIN_PATH="usr/local/sessionmanagerplugin/bin/session-manager-plugin"
  if command -v dpkg-deb >/dev/null 2>&1; then
    # Debian / Ubuntu — extract the .deb with dpkg-deb (always available).
    case "$ARCH" in
      x86_64) SSM_PKG="ubuntu_64bit" ;;
      aarch64) SSM_PKG="ubuntu_arm64" ;;
    esac
    curl --fail -L -o "$SCRIPT_TMPDIR/session-manager-plugin.deb" \
      "https://s3.amazonaws.com/session-manager-downloads/plugin/latest/${SSM_PKG}/session-manager-plugin.deb"
    dpkg-deb -x "$SCRIPT_TMPDIR/session-manager-plugin.deb" "$SCRIPT_TMPDIR"
  else
    # RPM-based distro — extract the .rpm with rpm2cpio + cpio.
    case "$ARCH" in
      x86_64) SSM_PKG="linux_64bit" ;;
      aarch64) SSM_PKG="linux_arm64" ;;
    esac
    curl --fail -L -o "$SCRIPT_TMPDIR/session-manager-plugin.rpm" \
      "https://s3.amazonaws.com/session-manager-downloads/plugin/latest/${SSM_PKG}/session-manager-plugin.rpm"
    rpm2cpio "$SCRIPT_TMPDIR/session-manager-plugin.rpm" | cpio -D "$SCRIPT_TMPDIR" -idm --quiet
  fi
  mv "$SCRIPT_TMPDIR/$SSM_BIN_PATH" "$LOCAL_BIN/"
fi

# AWS CLI v2. Official installer ships one bundle per arch.
curl --fail -L -o "$SCRIPT_TMPDIR/awscliv2.zip" \
  "https://awscli.amazonaws.com/awscli-exe-${OS}-${ARCH}.zip"
unzip -q "$SCRIPT_TMPDIR/awscliv2.zip" -d "$SCRIPT_TMPDIR"
"$SCRIPT_TMPDIR/aws/install" -i "$LOCAL_BIN/aws-cli" -b "$LOCAL_BIN" --update

rm -rf "${SCRIPT_TMPDIR:?}/"*
echo "AWS and SSM are now installed..."
