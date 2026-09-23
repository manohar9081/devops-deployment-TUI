#!/bin/bash

set -euo pipefail

# Requires bash >= 4 (associative arrays, fractional read timeouts).
# When started under an older bash (e.g. macOS's /bin/bash 3.2 via the
# shebang), re-exec under a newer bash if one is installed, so plain
# ./devops-deployment.sh keeps working.
if (( BASH_VERSINFO[0] < 4 )); then
  if [[ "${__DEVOPS_BASH_REEXEC:-}" == "1" ]]; then
    echo "This script needs bash >= 4 but no newer bash was found."
    echo "On macOS: brew install bash"
    exit 1
  fi
  NEW_BASH=""
  for candidate in /opt/homebrew/bin/bash /usr/local/bin/bash \
                   "$HOME/.linuxbrew/bin/bash" /usr/bin/bash; do
    if [[ -x "$candidate" ]] && "$candidate" -c '(( BASH_VERSINFO[0] >= 4 ))' 2>/dev/null; then
      NEW_BASH="$candidate"
      break
    fi
  done
  if [[ -z "$NEW_BASH" ]]; then
    path_bash="$(command -v bash 2>/dev/null || true)"
    if [[ -n "$path_bash" && "$path_bash" != "/bin/bash" ]] \
       && "$path_bash" -c '(( BASH_VERSINFO[0] >= 4 ))' 2>/dev/null; then
      NEW_BASH="$path_bash"
    fi
  fi
  if [[ -n "$NEW_BASH" ]]; then
    exec env __DEVOPS_BASH_REEXEC=1 "$NEW_BASH" "$0" "$@"
  fi
  echo "This script needs bash >= 4 but no newer bash was found."
  echo "On macOS: brew install bash"
  exit 1
fi

SCRIPT_DIR="$HOME/devops-deployment/scripts"
SCRIPT_TMPDIR="$HOME/devops-deployment/tmp"
LOCAL_BIN="$HOME/.local/bin"

# Tool name -> install script mapping (single source of truth).
declare -A TOOL_SCRIPTS=(
  [kubectl]="$SCRIPT_DIR/install-kubectl.sh"
  [k9s]="$SCRIPT_DIR/install-k9s.sh"
  [helm]="$SCRIPT_DIR/install-helm.sh"
  [terraform]="$SCRIPT_DIR/install-terraform.sh"
  [aws]="$SCRIPT_DIR/install-aws.sh"
  [brave]="$SCRIPT_DIR/install-brave.sh"
  [helium]="$SCRIPT_DIR/install-helium.sh"
  [bashtools]="$SCRIPT_DIR/install-bashtools.sh"
  [fzf-setup]="$SCRIPT_DIR/install-fzf-setup.sh"
  [oc]="$SCRIPT_DIR/install-oc.sh"
)

# ---------------------------------------------------------------
# Stack / cloud selection — asked once before the menu, persisted to
# config.env, re-runnable any time from the menu ("Config") or the
# "c" key. Drives the notes shown after the prerequisite checks and
# lets other scripts read DEVOPS_K8S_STACKS / DEVOPS_CLOUD_PROVIDERS.
# ---------------------------------------------------------------
STACK_CONFIG_FILE="$HOME/devops-deployment/config.env"
[[ -f "$STACK_CONFIG_FILE" ]] && source "$STACK_CONFIG_FILE"

K8S_STACK_ITEMS=(
  "rancher   — Rancher-managed clusters (kubeconfig from Rancher)"
  "openshift — Red Hat OpenShift (oc client)"
  "gke       — Google Kubernetes Engine (GCP)"
  "eks       — Amazon Elastic Kubernetes Service"
  "aks       — Azure Kubernetes Service"
  "plain     — plain kubeconfig / anything else"
)
CLOUD_ITEMS=(
  "aws    — Amazon Web Services"
  "gcp    — Google Cloud"
  "azure  — Microsoft Azure"
  "none   — on-prem / other / skip cloud tooling"
)

# first-token-of-each-selected-line -> space-separated string
tokens_from_selection() {
  local out="" line tok
  while IFS= read -r line; do
    tok="${line%%[[:space:]]*}"
    [[ -z "$tok" ]] && continue
    out="$out $tok"
  done <<<"$1"
  printf '%s' "${out# }"
}

ask_stack_cloud() {
  local tui="$HOME/devops-deployment/custom/misc/tui_select.sh"
  local sel stacks clouds

  # No interactive terminal: do not guess and do not overwrite existing config.
  if ! { exec 9<>/dev/tty; } 2>/dev/null; then
    echo "Stack/cloud selection needs a terminal — keeping existing config."
    return 0
  fi
  exec 9>&- 9<&-

  sel=$(printf '%s\n' "${K8S_STACK_ITEMS[@]}" \
        | bash "$tui" --multi \
          --title "Which Kubernetes stack(s) do you use? (Space to check, Enter to accept)" \
          --prompt "k8s> " ) || true
  if [[ -z "$sel" ]]; then
    echo "Cancelled — config unchanged."
    return 0
  fi
  stacks=$(tokens_from_selection "$sel")
  [[ -z "$stacks" ]] && stacks="rancher"

  sel=$(printf '%s\n' "${CLOUD_ITEMS[@]}" \
        | bash "$tui" --multi \
          --title "Which cloud provider(s) do you use? (Space to check, Enter to accept)" \
          --prompt "cloud> " ) || true
  if [[ -z "$sel" ]]; then
    echo "Cancelled — config unchanged."
    return 0
  fi
  clouds=$(tokens_from_selection "$sel")
  [[ -z "$clouds" ]] && clouds="none"

  DEVOPS_K8S_STACKS="$stacks"
  DEVOPS_CLOUD_PROVIDERS="$clouds"
  {
    echo "# Written by devops-deployment.sh — edit or re-run the menu's 'Config' entry."
    echo "export DEVOPS_K8S_STACKS=\"$stacks\""
    echo "export DEVOPS_CLOUD_PROVIDERS=\"$clouds\""
  } > "$STACK_CONFIG_FILE"
  echo "Saved: k8s stack(s) = $stacks · cloud = $clouds  ($STACK_CONFIG_FILE)"
  print_stack_notes
}

# One-line-per-selection notes shown after the prerequisite checks.
print_stack_notes() {
  local s
  [[ -n "${DEVOPS_K8S_STACKS:-}" ]] || return 0
  echo ""
  echo "Selected Kubernetes stack(s): ${DEVOPS_K8S_STACKS}"
  for s in ${DEVOPS_K8S_STACKS}; do
    case "$s" in
      rancher)    echo "  · rancher   : k8screds merges ~/k8sconfig/*.yaml (Rancher kubeconfigs) into ~/.kube/config" ;;
      openshift)  echo "  · openshift : install the oc client from the menu, then 'oc login --token=… --server=…'" ;;
      gke)        echo "  · gke       : needs gke-gcloud-auth-plugin — gcloud components install gke-gcloud-auth-plugin" ;;
      eks)        echo "  · eks       : login with 'aws eks update-kubeconfig --name <cluster> --region <region>'" ;;
      aks)        echo "  · aks       : login with 'az aks get-credentials --resource-group <rg> --name <cluster>'" ;;
      plain)      echo "  · plain     : drop kubeconfig YAMLs into ~/k8sconfig/ and run k8screds" ;;
    esac
  done
  if [[ -n "${DEVOPS_CLOUD_PROVIDERS:-}" ]]; then
    echo "Selected cloud provider(s): ${DEVOPS_CLOUD_PROVIDERS}"
    for s in ${DEVOPS_CLOUD_PROVIDERS}; do
      case "$s" in
        aws)   echo "  · aws   : AWS CLI + SSM plugin via the menu (option 5)" ;;
        gcp)   echo "  · gcp   : install Google Cloud SDK (gcloud) separately; brew install google-cloud-sdk on macOS" ;;
        azure) echo "  · azure : install az CLI separately; brew install azure-cli on macOS" ;;
        none)  : ;;
      esac
    done
  fi
}

# ---------------------------------------------------------------
# Detect OS
# ---------------------------------------------------------------
detect_os() {
  local uname_out
  uname_out="$(uname -s)"
  case "$uname_out" in
    Linux*)     echo "linux"  ;;
    Darwin*)    echo "macos"  ;;
    MINGW*|MSYS*|CYGWIN*|NT*) echo "windows" ;;
    *)          echo "unknown" ;;
  esac
}

OS="$(detect_os)"
echo "Detected OS: ${OS}"

# ---------------------------------------------------------------
# Package manager helper – returns 0 if a known manager is found
# and sets PKG_MANAGER.  Tries multiple managers in priority order.
# ---------------------------------------------------------------
detect_pkg_manager() {
  PKG_MANAGER=""
  case "$OS" in
    macos)
      if command -v brew &>/dev/null;  then PKG_MANAGER="brew";  return 0; fi
      if command -v port &>/dev/null;  then PKG_MANAGER="port";  return 0; fi
      ;;
    linux)
      if command -v apt-get &>/dev/null; then PKG_MANAGER="apt"; return 0; fi
      if command -v dnf &>/dev/null;     then PKG_MANAGER="dnf"; return 0; fi
      if command -v yum &>/dev/null;     then PKG_MANAGER="yum"; return 0; fi
      if command -v pacman &>/dev/null;  then PKG_MANAGER="pacman"; return 0; fi
      if command -v zypper &>/dev/null;   then PKG_MANAGER="zypper"; return 0; fi
      if command -v brew &>/dev/null;    then PKG_MANAGER="brew"; return 0; fi
      ;;
    windows)
      if command -v choco &>/dev/null;   then PKG_MANAGER="choco"; return 0; fi
      if command -v scoop &>/dev/null;   then PKG_MANAGER="scoop"; return 0; fi
      if command -v winget &>/dev/null;  then PKG_MANAGER="winget"; return 0; fi
      ;;
  esac
  return 1
}

# ---------------------------------------------------------------
#   install_pkg <cmd> <brew_pkg> <apt_pkg> <dnf_pkg> <choco_pkg>
#   Installs the package using the detected manager.
#   Falls back through: brew / apt / dnf / yum / pacman / zypper / choco / scoop / winget
# ---------------------------------------------------------------
install_pkg() {
  local cmd="$1"; shift
  local brew_pkg="$1"; shift
  local apt_pkg="$1"; shift
  local dnf_pkg="$1"; shift
  local choco_pkg="$1"; shift

  case "$PKG_MANAGER" in
    brew)   brew install "$brew_pkg" ;;
    port)   port install "$brew_pkg" ;;            # MacPorts uses same name usually
    apt)    apt-get update -qq && apt-get install -y "$apt_pkg" ;;
    dnf)    dnf install -y "$dnf_pkg" ;;
    yum)    yum install -y "$dnf_pkg" ;;
    pacman) pacman -Sy --noconfirm "$dnf_pkg" ;;
    zypper) zypper --non-interactive install "$dnf_pkg" ;;
    choco)  choco install "$choco_pkg" -y ;;
    scoop)  scoop install "$choco_pkg" ;;
    winget) winget install --accept-source-agreements --accept-package-agreements "$choco_pkg" ;;
  esac
}

# ---------------------------------------------------------------
# Prerequisites – tools required by the install scripts themselves
# ---------------------------------------------------------------
install_if_missing() {
  local cmd="$1"; shift
  local brew_pkg="$1"; shift
  local apt_pkg="$1"; shift
  local dnf_pkg="$1"; shift
  local choco_pkg="$1"; shift

  if command -v "$cmd" &>/dev/null; then
    echo "✓ ${cmd} is installed."
    return 0
  fi

  echo "⚠ ${cmd} is not installed."

  if ! detect_pkg_manager; then
    echo "ERROR: ${cmd} is required but not found, and no supported package manager was detected."
    echo "       Please install ${cmd} manually, then re-run this script."
    return 1
  fi

  echo "   Installing ${cmd} via ${PKG_MANAGER}..."
  install_pkg "$cmd" "$brew_pkg" "$apt_pkg" "$dnf_pkg" "$choco_pkg"
}

echo ""
echo "Checking prerequisites..."

# Format: cmd brew_pkg apt_pkg dnf_pkg choco_pkg
#          (dnf_pkg is also used for yum / pacman / zypper)
REQUIRED_TOOLS=(
  "jq     jq     jq     jq     jq"
  "unzip  unzip  unzip  unzip  unzip"
  "wget   wget   wget   wget   wget"
)

for entry in "${REQUIRED_TOOLS[@]}"; do
  install_if_missing $entry
done

print_stack_notes

echo ""

# Ensure $LOCAL_BIN is on PATH.
if [[ ":$PATH:" != *":$LOCAL_BIN:"* ]]; then
  CONFIG_FILE=""
  if [[ -f "$HOME/.bashrc" ]]; then
    CONFIG_FILE="$HOME/.bashrc"
  elif [[ -f "$HOME/.zshrc" ]]; then
    CONFIG_FILE="$HOME/.zshrc"
  else
    echo "No compatible shell config file found."
    exit 1
  fi
  echo "$LOCAL_BIN is not in the PATH. Adding it..."
  echo "export PATH=\"$LOCAL_BIN:\$PATH\"" >> "$CONFIG_FILE"
  source "$CONFIG_FILE"
  echo "$LOCAL_BIN has been added to the PATH."
else
  echo "$LOCAL_BIN is already in the PATH."
fi

mkdir -p "$SCRIPT_TMPDIR"

run_install() {
  local tool="$1"
  local script="${TOOL_SCRIPTS[$tool]}"
  if [[ -z "$script" || ! -f "$script" ]]; then
    echo "ERROR: install script for '$tool' not found ($script)."
    return 1
  fi
  echo ">>> Installing ${tool}..."
  bash "$script"
}

# ===============================================================
# =================  TUI (CleanMyMac style)  ====================
# ===============================================================

setup_colors() {
  if [[ -t 1 && -z "${NO_COLOR:-}" ]]; then
    C_RESET=$'\e[0m';  C_BOLD=$'\e[1m';    C_DIM=$'\e[2m'
    C_MAGENTA=$'\e[35m'; C_BMAGENTA=$'\e[1;35m'
    C_GREEN=$'\e[32m';  C_BLUE=$'\e[34m';  C_YELLOW=$'\e[33m'; C_CYAN=$'\e[36m'
    C_GRAY=$'\e[90m';   C_WHITE=$'\e[97m'
    C_BGREEN=$'\e[1;32m'; C_BRED=$'\e[1;31m'; C_BYELLOW=$'\e[1;33m'
  else
    C_RESET=''; C_BOLD=''; C_DIM=''
    C_MAGENTA=''; C_BMAGENTA=''
    C_GREEN=''; C_BLUE=''; C_YELLOW=''; C_CYAN=''
    C_GRAY=''; C_WHITE=''
    C_BGREEN=''; C_BRED=''; C_BYELLOW=''
  fi
}

# Menu data: label | description | color group | action
MENU_LABELS=(
  "Install Kubectl"      "Install K9s"          "Install Helm"
  "Install Terraform"    "Install AWS CLI"      "Install Brave"
  "Install Helium"       "Install Bash tools"   "Setup fzf + cursor"
  "Install everything"   "Install core only"    "Install oc (OpenShift)"
  "Config: stacks & cloud"  "Update installed scripts"
)
MENU_DESCS=(
  "k8s CLI + kubectx/kubens"   "k8s terminal UI"            "k8s package manager"
  "infrastructure as code"     "aws-cli v2 + SSM plugin"    "browser (AppImage)"
  "browser (AppImage)"         "dotfiles + aliases + k cmd" "interactive search (opt-in)"
  "runs 1-9 in order"          "skips browsers & fzf setup" "OpenShift/kubectl client"
  "choose k8s stack + cloud"   "re-copy custom/ to ~/.local/bin"
)
MENU_GROUPS=( k8s k8s k8s cloud cloud app app app setup bulk bulk k8s setup setup )
MENU_ACTIONS=( kubectl k9s helm terraform aws brave helium bashtools fzf-setup __all __core oc __config __update_scripts )
N_ITEMS=${#MENU_LABELS[@]}

group_color() {
  case "$1" in
    k8s)   echo "$C_GREEN"  ;;
    cloud) echo "$C_BLUE"   ;;
    app)   echo "$C_YELLOW" ;;
    setup) echo "$C_MAGENTA" ;;
    bulk)  echo "$C_CYAN"   ;;
  esac
}

tui_geometry() {
  TUI_COLS=$(tput cols 2>/dev/null || echo 80)
  TUI_LINES=$(tput lines 2>/dev/null || echo 24)
  if (( TUI_COLS > 78 )); then TUI_COLS=78; fi
  LABEL_W=22
  SHOW_LOGO=1
  # NB: plain `(( x < n )) && SHOW_LOGO=0` as the last line would make the
  # function return 1 on tall terminals and set -e would kill the script
  # before the menu is drawn.
  if (( TUI_LINES < 27 )); then SHOW_LOGO=0; fi
}

tui_use_tui() {
  [[ -t 0 && -t 1 ]] && (( TUI_LINES >= 16 && TUI_COLS >= 60 ))
}

# Draw the whole menu; cursor ends one line below the block.
# Sets BLOCK_HEIGHT so the caller can erase/redraw in place.
tui_draw() {
  local sel="$1"
  local i num pad label desc gcol marker numstr line
  local -a OUT=()

  if [[ "$SHOW_LOGO" == 1 ]]; then
    OUT+=("$(printf '%s%s' "$C_BMAGENTA" ' ██████╗ ███████╗██████╗ ██████╗  ██████╗ ██╗  ██╗' "$C_RESET")")
    OUT+=("$(printf '%s%s' "$C_BMAGENTA" ' ██╔══██╗██╔════╝██╔══██╗██╔══██╗██╔═══██╗╚██╗██╔╝' "$C_RESET")")
    OUT+=("$(printf '%s%s' "$C_BMAGENTA" ' ██║  ██║███████╗██████╔╝██║  ██║██║   ██║ ╚███╔╝ ' "$C_RESET")")
    OUT+=("$(printf '%s%s' "$C_BMAGENTA" ' ██║  ██║╚════██║██╔══██╗██║  ██║██║   ██║ ██╔██╗ ' "$C_RESET")")
    OUT+=("$(printf '%s%s' "$C_BMAGENTA" ' ██████╔╝███████║██║  ██║██████╔╝╚██████╔╝██╔╝ ██╗' "$C_RESET")")
    OUT+=("$(printf '%s%s' "$C_BMAGENTA" ' ╚═════╝ ╚══════╝╚═╝  ╚═╝╚═════╝ ╚═════╝ ╚═╝  ╚═╝' "$C_RESET")")
    OUT+=("$(printf '%s%s%s' "$C_MAGENTA" "D E P L O Y M E N T" "$C_RESET")")
  else
    OUT+=("$(printf '%s%s%s' "$C_BMAGENTA" "⬢ devops-deployment" "$C_RESET")")
  fi
  OUT+=("")

  # Status line: prerequisites + os + package manager.
  local pkg_part="pkg: none"
  [[ -n "${PKG_MANAGER:-}" ]] && pkg_part="pkg: ${PKG_MANAGER}"
  OUT+=("$(printf '%s✓ prerequisites ready%s   %sos: %s · %s%s' \
    "$C_BGREEN" "$C_RESET" "$C_DIM" "$OS" "$pkg_part" "$C_RESET")")
  OUT+=("")

  for i in "${!MENU_LABELS[@]}"; do
    num=$((i + 1))
    numstr=$(printf '%2d' "$num")
    gcol=$(group_color "${MENU_GROUPS[$i]}")
    if (( i == sel )); then
      marker=$(printf '%s▶%s' "$C_BMAGENTA" "$C_RESET")
      label=$(printf '%s%s%s' "$C_BOLD" "${MENU_LABELS[$i]}" "$C_RESET")
      desc=$(printf '%s%s' "$C_GRAY" "${MENU_DESCS[$i]}")
      numstr=$(printf '%s%s%s' "$C_BMAGENTA" "$numstr" "$C_RESET")
    else
      marker=" "
      label=$(printf '%s%s%s' "$gcol" "${MENU_LABELS[$i]}" "$C_RESET")
      desc=$(printf '%s%s%s' "$C_DIM" "${MENU_DESCS[$i]}" "$C_RESET")
      numstr=$(printf '%s%s%s' "$C_DIM" "$numstr" "$C_RESET")
    fi
    pad=$(( LABEL_W - ${#MENU_LABELS[$i]} ))
    [[ $pad -lt 1 ]] && pad=1
    OUT+=("$(printf '%s %s  %s%*s%s' "$marker" "$numstr" "$label" "$pad" "" "$desc")")
  done

  OUT+=("")
  OUT+=("$(printf '%s%s%s' "$C_MAGENTA" "$(printf '━%.0s' $(seq 1 "$TUI_COLS"))" "$C_RESET")")
  OUT+=("$(printf '%s↑↓%s navigate   %s⏎%s run   %s1-9%s select   %sc%s config   %su%s update   %sq%s quit' \
    "$C_BMAGENTA" "$C_RESET" "$C_BMAGENTA" "$C_RESET" "$C_BMAGENTA" "$C_RESET" "$C_BMAGENTA" "$C_RESET" "$C_BMAGENTA" "$C_RESET" "$C_BMAGENTA" "$C_RESET")")
  OUT+=("$(printf '%sdevops-deployment · interactive menu%s' "$C_DIM" "$C_RESET")")

  for line in "${OUT[@]}"; do
    printf '\r\e[2K%s\n' "$line"
  done
  BLOCK_HEIGHT=${#OUT[@]}
}

# Erase the menu block (cursor must be one line below it).
tui_erase() {
  printf '\e[%dA' "$BLOCK_HEIGHT"
  local i
  for (( i = 0; i < BLOCK_HEIGHT; i++ )); do
    printf '\r\e[2K\n'
  done
  printf '\e[%dA' "$BLOCK_HEIGHT"
}

tui_redraw() {
  local sel="$1"
  printf '\e[%dA' "$BLOCK_HEIGHT"
  tui_draw "$sel"
}

run_menu_action() {
  local sel="$1"
  local action="${MENU_ACTIONS[$sel]}"
  local -a tools=()

  case "$action" in
    __all) tools=(kubectl k9s helm terraform aws brave helium bashtools fzf-setup) ;;
    __core) tools=(kubectl k9s helm terraform aws bashtools) ;;
    __config)
      ask_stack_cloud
      return 0
      ;;
    __update_scripts)
      bash "$SCRIPT_DIR/update-scripts.sh"
      return 0
      ;;
    *) tools=("$action") ;;
  esac

  if [[ "$action" == __all || "$action" == __core ]]; then
    printf '\n%s?%s %sThis will install %d tools. Continue? [y/N]%s ' \
      "$C_BYELLOW" "$C_RESET" "$C_BOLD" "${#tools[@]}" "$C_RESET"
    local confirm
    read -r confirm
    if [[ "$confirm" != "y" && "$confirm" != "Y" ]]; then
      echo "Aborted."
      return 0
    fi
  fi

  local tool rc=0
  for tool in "${tools[@]}"; do
    if run_install "$tool"; then
      printf '%s✓%s %s done\n' "$C_BGREEN" "$C_RESET" "$tool"
    else
      printf '%s✗%s %s FAILED\n' "$C_BRED" "$C_RESET" "$tool"
      rc=1
    fi
  done
  return "$rc"
}

tui_run() {
  local sel="$1"
  tui_erase
  printf '%s▶%s %s%s%s\n\n' \
    "$C_BMAGENTA" "$C_RESET" "$C_BOLD" "${MENU_LABELS[$sel]}" "$C_RESET"

  printf '\e[?25h'                      # installer prompts need a visible cursor
  run_menu_action "$sel" || true
  printf '\e[?25l'

  printf '\n%s─ press any key to return to the menu ─%s' "$C_DIM" "$C_RESET"
  local ignored
  read -rsn1 ignored || true
  printf '\n'
  clear
}

# Plain numeric prompt – used when stdin/stdout is not a TTY or the
# terminal is too small for the interactive layout.
fallback_numeric_menu() {
  echo "Select an option:"
  local i
  for i in "${!MENU_LABELS[@]}"; do
    printf '  %2d - %s\n' "$((i + 1))" "${MENU_LABELS[$i]}"
  done
  echo ""
  read -rp "Enter option: " OPTION

  local sel
  case "$OPTION" in
    1|2|3|4|5|6|7|8|9) sel=$(( OPTION - 1 )) ;;
    10|11|12|13|14) sel=$(( OPTION - 1 )) ;;
    *)
      echo "Invalid option: '${OPTION}'."
      exit 1
      ;;
  esac
  run_menu_action "$sel" || true
}

tui_loop() {
  local sel=0 key seq d
  printf '\e[?25l'                      # hide cursor while navigating
  trap 'printf "\e[?25h"' EXIT
  clear
  tui_draw "$sel"

  while true; do
    IFS= read -rsn1 key || key='q'
    case "$key" in
      $'\e')
        seq=''
        IFS= read -rsn2 -t 0.05 seq || true
        case "$seq" in
          '[A') sel=$(( (sel - 1 + N_ITEMS) % N_ITEMS )) ;;
          '[B') sel=$(( (sel + 1) % N_ITEMS )) ;;
          '[H'|'1~'|'7~') sel=0 ;;
          '[F'|'4~'|'8~') sel=$(( N_ITEMS - 1 )) ;;
        esac
        tui_redraw "$sel"
        ;;
      j|J) sel=$(( (sel + 1) % N_ITEMS )); tui_redraw "$sel" ;;
      k|K) sel=$(( (sel - 1 + N_ITEMS) % N_ITEMS )); tui_redraw "$sel" ;;
      g)   sel=0; tui_redraw "$sel" ;;
      G)   sel=$(( N_ITEMS - 1 )); tui_redraw "$sel" ;;
      $'\n'|$'\r'|'') tui_run "$sel"; tui_draw "$sel" ;;
      [1-9])
        d=$(( key - 1 ))
        if (( d == sel )); then
          tui_run "$sel"; tui_draw "$sel"
        else
          sel=$d; tui_redraw "$sel"
        fi
        ;;
      0)
        if (( sel == 9 )); then tui_run "$sel"; tui_draw "$sel"
        else sel=9; tui_redraw "$sel"; fi
        ;;
      c|C)
        tui_erase
        ask_stack_cloud
        printf '\n%spress any key to return%s' "$C_DIM" "$C_RESET"
        read -rsn1 || true
        clear
        tui_draw "$sel"
        ;;
      u|U)
        tui_erase
        bash "$SCRIPT_DIR/update-scripts.sh"
        printf '\n%spress any key to return%s' "$C_DIM" "$C_RESET"
        read -rsn1 || true
        clear
        tui_draw "$sel"
        ;;
      q|Q|$'\004') break ;;
    esac
  done

  tui_erase
  printf '\e[?25h'
  trap - EXIT
}

# ===============================================================
# =======================  MAIN  ================================
# ===============================================================

setup_colors
tui_geometry
detect_pkg_manager || true

if [[ ! -f "$STACK_CONFIG_FILE" ]]; then
  ask_stack_cloud
  echo ""
fi

if tui_use_tui; then
  tui_loop
else
  fallback_numeric_menu
fi

echo "Done. Please reopen all terminals."
