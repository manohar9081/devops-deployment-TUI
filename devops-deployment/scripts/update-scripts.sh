#!/bin/bash

set -euo pipefail

# Re-copy the helper scripts from the deployment repo (~/devops-deployment/
# custom/) into ~/.local/bin/scripts/ — so edits made in the repo (e.g. a
# fixed add_yaml_to_kconf.sh) take effect for the aliases (k8screds, k_top,
# ssh_key_gen, ...) without re-running the full Bash-tools installer.
#
# Dotfiles (~/.bashrc, aliases, functions) are NOT touched here — refresh
# those with menu option 8 (install-bashtools.sh) if they changed.

SRC="$HOME/devops-deployment/custom"
DST="$HOME/.local/bin/scripts"

if [[ ! -d "$SRC" ]]; then
  echo "Source folder not found: $SRC"
  echo "This updater expects the deployment repo at ~/devops-deployment."
  exit 1
fi

mkdir -p "$DST"

echo "Comparing $SRC -> $DST ..."
# List everything that differs or is new; ignore stale files that exist only
# in the installed copy (we never delete anything the user may have added).
change_list=$(diff -rq "$SRC" "$DST" 2>/dev/null | grep -vF "Only in $DST" || true)

if [[ -z "$change_list" ]]; then
  echo "Already up to date — nothing to copy."
  exit 0
fi

printf '%s\n' "$change_list" | sed 's/^/  /'

cp -R "$SRC"/. "$DST"/

n=$(printf '%s\n' "$change_list" | wc -l)
echo ""
echo "Updated $n item(s). New/changed scripts take effect immediately"
echo "(new ALIASES need a fresh terminal, or menu option 8 for dotfiles)."
