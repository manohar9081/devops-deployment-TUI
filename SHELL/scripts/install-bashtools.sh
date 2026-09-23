#!/bin/bash

set -euo pipefail

LOCAL_BIN="$HOME/.local/bin"
SCRIPT_DIR="$HOME/devops-deployment/scripts"
CUSTOM_DIR="$HOME/devops-deployment/custom"
SCRIPT_TMPDIR="$HOME/devops-deployment/tmp"
BASHFILES_DIR="$HOME/devops-deployment/bash-files"
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

mkdir -p "$SCRIPT_TMPDIR" "$SCRIPT_DIR" "$LOCAL_BIN/scripts"

if [[ "$ARCH" == "x86_64" ]]; then
  ARCH="amd64"
elif [[ "$ARCH" == "aarch64" || "$ARCH" == "arm64" ]]; then
  ARCH="arm64"
else
  echo "Unsupported architecture: $ARCH"
  exit 1
fi

# ---- Install fzf ----
if ! command -v fzf >/dev/null 2>&1; then
  VERSION=$(curl -fsSL https://api.github.com/repos/junegunn/fzf/releases/latest | jq -r .tag_name | sed 's/^v//')
  DOWNLOAD_URL="https://github.com/junegunn/fzf/releases/download/v$VERSION/fzf-${VERSION}-${OS}_${ARCH}.tar.gz"
  echo "Downloading fzf from: $DOWNLOAD_URL"

  curl --fail -L -o "$SCRIPT_TMPDIR/fzf-$VERSION.tar.gz" "$DOWNLOAD_URL"
  # Extract to a /tmp dir (mktemp -d): on some VM/shared-folder filesystems
  # tar's file creation fails with "Function not implemented"; /tmp works.
  EXTRACT_DIR="$(mktemp -d)"
  tar -xzf "$SCRIPT_TMPDIR/fzf-$VERSION.tar.gz" -C "$EXTRACT_DIR"
  cp -r "$EXTRACT_DIR/fzf" "$LOCAL_BIN/"
  chmod +x "$LOCAL_BIN/fzf"
  rm -rf "$EXTRACT_DIR"
fi

backup_file() {
  local target="$1"
  if [[ -f "$target" ]]; then
    local bkp="${target}_bkp_$(date +%Y%m%d%H%M)"
    cp -p "$target" "$bkp"
    echo "Backed up existing $target -> $bkp"
  fi
}

cp -r "$CUSTOM_DIR"/* "$LOCAL_BIN/scripts/"

backup_file "$HOME/.bash_function"
cp -r "$BASHFILES_DIR/bash_function" "$HOME/.bash_function"

backup_file "$HOME/.bash_environment"
cp -r "$BASHFILES_DIR/bash_environment" "$HOME/.bash_environment"

backup_file "$HOME/.aws_environment"
cp -r "$BASHFILES_DIR/aws_environment" "$HOME/.aws_environment"

backup_file "$HOME/.commands_environment"
cp -r "$BASHFILES_DIR/commands_environment" "$HOME/.commands_environment"

backup_file "$HOME/.bashrc"
cp -r "$BASHFILES_DIR/bashrc" "$HOME/.bashrc"

# ---- Create symlink to AWS config (ensure ~/.aws exists) ----
mkdir -p "$HOME/.aws"
if [[ -e "$HOME/.aws/config" && ! -L "$HOME/.aws/config" ]]; then
  backup_file "$HOME/.aws/config"
fi
ln -sf "$LOCAL_BIN/scripts/config" "$HOME/.aws/config"

# ---- Set executable permission on scripts (recursive — scripts now live in
#      aws/ k8s/ tf/ misc/ subfolders under $LOCAL_BIN/scripts/) ----
find "$LOCAL_BIN/scripts" -type f -name '*.sh' -exec chmod +x {} +

rm -rf "${SCRIPT_TMPDIR:?}/"*
echo "Installed Bash Tools..."
