#!/bin/sh
set -e

# beats installer - detects OS/arch and downloads the right binary

REPO="bierlingm/beats"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"

# Detect OS
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$OS" in
    darwin) OS="darwin" ;;
    linux) OS="linux" ;;
    *) echo "Unsupported OS: $OS"; exit 1 ;;
esac

# Detect architecture
ARCH=$(uname -m)
case "$ARCH" in
    x86_64|amd64) ARCH="amd64" ;;
    arm64|aarch64) ARCH="arm64" ;;
    *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

# Get latest version
VERSION=$(curl -sL "https://api.github.com/repos/$REPO/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')
if [ -z "$VERSION" ]; then
    echo "Failed to get latest version"
    exit 1
fi

echo "Installing beats $VERSION for $OS/$ARCH..."

# Download and extract
TARBALL="beats_${VERSION#v}_${OS}_${ARCH}.tar.gz"
URL="https://github.com/$REPO/releases/download/$VERSION/$TARBALL"

TMPDIR=$(mktemp -d)
trap "rm -rf $TMPDIR" EXIT

curl -sL "$URL" | tar xz -C "$TMPDIR"

# Install
if [ -w "$INSTALL_DIR" ]; then
    mv "$TMPDIR/beats" "$INSTALL_DIR/beats"
else
    echo "Installing to $INSTALL_DIR (requires sudo)..."
    sudo mv "$TMPDIR/beats" "$INSTALL_DIR/beats"
fi

echo "beats $VERSION installed to $INSTALL_DIR/beats"
echo ""
echo "Get started:"
echo "  beats add \"Your first beat\""
echo "  beats list"
