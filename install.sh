#!/bin/sh
set -e

# hcli install script — downloads the correct binary from GitHub Releases

REPO="justinmaks/hedge-local"
INSTALL_DIR="/usr/local/bin"

# Detect OS
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$OS" in
    linux*)  OS="linux" ;;
    darwin*) OS="darwin" ;;
    *)       echo "Unsupported OS: $OS"; exit 1 ;;
esac

# Detect arch
ARCH=$(uname -m)
case "$ARCH" in
    x86_64|amd64) ARCH="amd64" ;;
    arm64|aarch64) ARCH="arm64" ;;
    *)             echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

# Get latest release URL
LATEST_URL="https://github.com/${REPO}/releases/latest"
ARCHIVE_NAME="hcli_${OS}_${ARCH}.tar.gz"
DOWNLOAD_URL="${LATEST_URL}/download/${ARCHIVE_NAME}"

echo "Downloading hcli for ${OS}/${ARCH}..."
TMPDIR=$(mktemp -d)
trap "rm -rf $TMPDIR" EXIT

if command -v curl >/dev/null 2>&1; then
    curl -fsSL -o "${TMPDIR}/${ARCHIVE_NAME}" "${DOWNLOAD_URL}"
elif command -v wget >/dev/null 2>&1; then
    wget -q -O "${TMPDIR}/${ARCHIVE_NAME}" "${DOWNLOAD_URL}"
else
    echo "Error: neither curl nor wget is installed"
    exit 1
fi

echo "Extracting..."
tar -xzf "${TMPDIR}/${ARCHIVE_NAME}" -C "${TMPDIR}"

echo "Installing to ${INSTALL_DIR}/hcli..."
if [ -w "${INSTALL_DIR}" ]; then
    cp "${TMPDIR}/hcli" "${INSTALL_DIR}/hcli"
else
    sudo cp "${TMPDIR}/hcli" "${INSTALL_DIR}/hcli"
fi
chmod +x "${INSTALL_DIR}/hcli"

echo ""
echo "hcli installed successfully!"
echo ""
echo "Next steps:"
echo "  hcli setup claude     # configure Claude Code telemetry"
echo "  hcli setup opencode   # configure OpenCode telemetry"
echo "  hcli                  # start TUI with embedded receiver"
echo ""