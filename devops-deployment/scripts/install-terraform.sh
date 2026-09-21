#!/bin/bash

set -euo pipefail

LOCAL_BIN="$HOME/.local/bin"
TERRAFORM_VERSION_DOWNLOAD="$HOME/.local/bin/terraform-versions"
SCRIPT_DIR="$HOME/devops-deployment/scripts"
SCRIPT_TMPDIR="$HOME/devops-deployment/tmp"
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

mkdir -p "$SCRIPT_TMPDIR" "$SCRIPT_DIR" "$TERRAFORM_VERSION_DOWNLOAD"

get_latest_version() {
  VERSION=$(curl -fsSL https://api.github.com/repos/hashicorp/terraform/releases/latest | jq -r .tag_name | sed 's/^v//')
}

get_last_5_stable_versions() {
  VERSIONS=$(curl -fsSL https://api.github.com/repos/hashicorp/terraform/releases \
    | jq -r 'map(select(.prerelease == false))
         | map(.tag_name | ltrimstr("v"))
         | sort_by(split(".") | map(tonumber? // 0))
         | reverse
         | .[0:5]
         | .[]' \
    | sed 's/^v//')

  echo "The last 5 stable terraform versions are:"
  echo "$VERSIONS"
}
get_last_5_stable_versions

echo "Enter the terraform version you want to download (press Enter for the latest version):"
read -r USER_VERSION

if [[ -z "$USER_VERSION" ]]; then
  echo "No version specified, fetching the latest stable version..."
  get_latest_version
else
  VERSION="$USER_VERSION"
  echo "Downloading terraform version $VERSION..."
fi

if [[ "$ARCH" == "x86_64" ]]; then
  ARCH="amd64"
elif [[ "$ARCH" == "aarch64" || "$ARCH" == "arm64" ]]; then
  ARCH="arm64"
else
  echo "Unsupported architecture: $ARCH"
  exit 1
fi

DOWNLOAD_URL="https://releases.hashicorp.com/terraform/$VERSION/terraform_${VERSION}_${OS}_${ARCH}.zip"
echo "Downloading terraform from: $DOWNLOAD_URL"

curl --fail -L -o "$SCRIPT_TMPDIR/terraform_$VERSION.zip" "$DOWNLOAD_URL"

echo "Extracting terraform_$VERSION.zip..."
unzip -q "$SCRIPT_TMPDIR/terraform_$VERSION.zip" -d "$SCRIPT_TMPDIR"

echo "Copying terraform binary to $TERRAFORM_VERSION_DOWNLOAD..."
cp "$SCRIPT_TMPDIR/terraform" "$TERRAFORM_VERSION_DOWNLOAD/terraform-$VERSION"
chmod +x "$TERRAFORM_VERSION_DOWNLOAD/terraform-$VERSION"

ln -sf "$TERRAFORM_VERSION_DOWNLOAD/terraform-$VERSION" "$LOCAL_BIN/terraform"

rm -rf "${SCRIPT_TMPDIR:?}/"*
echo "Terraform is now installed..."
