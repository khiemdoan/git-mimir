#!/bin/sh
set -e

REPO="thuongh2/git-mimir"

# Detect OS
case "$(uname -s)" in
    Linux*)  OS=linux;;
    Darwin*) OS=darwin;;
    MINGW*|MSYS*|CYGWIN*) OS=windows;;
    *)       echo "Unsupported OS: $(uname -s)"; exit 1;;
esac

# Detect architecture
case "$(uname -m)" in
    x86_64)  ARCH=amd64;;
    amd64)   ARCH=amd64;;
    arm64)   ARCH=arm64;;
    aarch64) ARCH=arm64;;
    *)       echo "Unsupported architecture: $(uname -m)"; exit 1;;
esac

# Windows uses zip
if [ "$OS" = "windows" ]; then
    FILENAME="mimir-${OS}-${ARCH}.zip"
    URL="https://github.com/${REPO}/releases/download/${VERSION}/${FILENAME}"
    echo "Downloading mimir ${VERSION} for ${OS}/${ARCH}..."
    TMPDIR=$(mktemp -d)
    trap 'rm -rf "$TMPDIR"' EXIT
    if ! curl -fL "$URL" -o "${TMPDIR}/${FILENAME}"; then
        echo "Error: Failed to download the release archive from $URL" >&2
        exit 1
    fi
    unzip -q "${TMPDIR}/${FILENAME}" -d "$TMPDIR"
    # Install binary
    INSTALL_DIR="/usr/local/bin"
    if [ -w "$INSTALL_DIR" ]; then
        mv "${TMPDIR}/mimir.exe" "${INSTALL_DIR}/mimir.exe"
    else
        echo "Installing to ${INSTALL_DIR} (requires sudo)..."
        sudo mv "${TMPDIR}/mimir.exe" "${INSTALL_DIR}/mimir.exe"
    fi
    echo "mimir ${VERSION} installed to ${INSTALL_DIR}/mimir.exe"
    mimir.exe --version
    exit 0
fi

# Get latest version
VERSION=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')
if [ -z "$VERSION" ]; then
    echo "Error: could not determine latest version"
    exit 1
fi

FILENAME="mimir-${OS}-${ARCH}.tar.gz"
URL="https://github.com/${REPO}/releases/download/${VERSION}/${FILENAME}"

echo "Downloading mimir ${VERSION} for ${OS}/${ARCH}..."
TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

if ! curl -fL "$URL" -o "${TMPDIR}/${FILENAME}"; then
    echo "Error: Failed to download the release archive from $URL" >&2
    echo "Please check your network connection or verify that a release exists for ${OS}/${ARCH}." >&2
    exit 1
fi
tar xzf "${TMPDIR}/${FILENAME}" -C "$TMPDIR"

# Verify binary was extracted
if [ ! -f "${TMPDIR}/mimir" ]; then
    echo "Error: Binary not found in archive. Contents:" >&2
    tar tzf "${TMPDIR}/${FILENAME}" >&2
    exit 1
fi

# Install binary
INSTALL_DIR="/usr/local/bin"
if [ -w "$INSTALL_DIR" ]; then
    mv "${TMPDIR}/mimir" "${INSTALL_DIR}/mimir"
else
    echo "Installing to ${INSTALL_DIR} (requires sudo)..."
    sudo mv "${TMPDIR}/mimir" "${INSTALL_DIR}/mimir"
fi

echo "mimir ${VERSION} installed to ${INSTALL_DIR}/mimir"
mimir --version
