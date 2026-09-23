#!/bin/bash

set -euo pipefail

# Self-locate the checkout that holds this installer (assets/scripts/../..),
# like update-scripts.sh does — never assume the bash-era
# $HOME/devops-deployment path; a checkout can live anywhere. When the Go
# menu runs this installer from the materialized embedded assets, the
# environment it exports wins: ROOT/SCRIPT_DIR/SCRIPT_TMPDIR/BASHFILES_DIR
# name the materialized tree and the resolved deployment root, and
# DEVOPS_SELF names the running binary — the BASH_SOURCE-derived defaults
# below keep direct bash-from-repo usage working.
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")/../.." && pwd -P)"
LOCAL_BIN="$HOME/.local/bin"
SCRIPT_DIR="${SCRIPT_DIR:-$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" && pwd -P)}"
SCRIPT_TMPDIR="${SCRIPT_TMPDIR:-$ROOT/tmp}"
BASHFILES_DIR="${BASHFILES_DIR:-$ROOT/assets/bash-files}"
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

# ---- Helper scripts (the old custom/ copy line) ----
# Nothing is copied here anymore: the maintained custom-scripts tree is
# embedded in the Go binary and deployed by the sync half of
# `devops update` (menu.UpdateScripts → customsync.Sync), which mirrors it
# into $LOCAL_BIN/scripts — run `devops update` once after this installer
# (or use the menu's "Update installed scripts" entry).

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

# ---- Install the Go entrypoint as a portable copy on the PATH (`devops`) ----
# A copy, not a symlink: the binary resolves its checkout through the pin
# file, and `devops update` can refresh the copy in place. The source is
# DEVOPS_SELF when the Go menu set it (the running binary itself, since it
# carries the embedded assets); the fallback keeps direct
# bash-from-repo usage working, where the binary sits at
# ../../GO/devops-deployment-go (assets/scripts/ → repo root → GO).
# tmp-file + mv -f (rename) swaps the command atomically, so a running
# `devops` binary is never clobbered mid-execution. Missing binary: note
# it and move on.
GO_BIN="${DEVOPS_SELF:-$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")/../.." && pwd -P)/GO/devops-deployment-go}"
if [[ -x "$GO_BIN" ]]; then
  cp "$GO_BIN" "$LOCAL_BIN/.devops.tmp.$$"
  mv -f "$LOCAL_BIN/.devops.tmp.$$" "$LOCAL_BIN/devops"
  chmod 0755 "$LOCAL_BIN/devops"
  # Pin the checkout this installer ran from — but only when running
  # standalone. Under the Go menu (DEVOPS_SELF set) the binary owns the
  # pin: resolveRoot persists it whenever a marker candidate wins.
  if [[ -z "${DEVOPS_SELF:-}" ]]; then
    ROOT="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")/../.." && pwd -P)"
    mkdir -p "$HOME/.config/devops-deployment"
    printf '%s\n' "$ROOT" > "$HOME/.config/devops-deployment/path"
  fi
else
  echo "Go binary not found — 'devops' not installed (build it: cd GO && go build -o devops-deployment-go .)"
fi

# ---- Set executable permission on scripts (recursive — scripts now live in
#      aws/ k8s/ tf/ misc/ subfolders under $LOCAL_BIN/scripts/) ----
find "$LOCAL_BIN/scripts" -type f -name '*.sh' -exec chmod +x {} +

rm -rf "${SCRIPT_TMPDIR:?}/"*
echo "Installed Bash Tools..."
