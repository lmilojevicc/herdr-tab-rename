#!/usr/bin/env bash
# Download the prebuilt herdr-tab-rename binary for the current platform and
# verify its SHA256 checksum. Runs as the plugin's [[build]] step during
# `herdr plugin install` so end users do NOT need Go installed.
set -euo pipefail

# --- overrides (for testing/advanced use) ---
VERSION="${HERDR_TAB_RENAME_VERSION:-latest}"
BASE="${HERDR_TAB_RENAME_BASE:-https://github.com/lmilojevicc/herdr-tab-rename/releases}"

# --- platform detection ---
case "$(uname -s | tr '[:upper:]' '[:lower:]')" in
  linux)  OS=linux ;;
  darwin) OS=darwin ;;
  *) echo "install.sh: unsupported OS: $(uname -s)" >&2; exit 1 ;;
esac

case "$(uname -m)" in
  x86_64|amd64)  ARCH=amd64 ;;
  arm64|aarch64) ARCH=arm64 ;;
  *) echo "install.sh: unsupported arch: $(uname -m)" >&2; exit 1 ;;
esac

ASSET="herdr-tab-rename-${OS}-${ARCH}"

if [ "$VERSION" = "latest" ]; then
  BIN_URL="${BASE}/latest/download/${ASSET}"
  SUM_URL="${BASE}/latest/download/SHA256SUMS"
else
  BIN_URL="${BASE}/download/${VERSION}/${ASSET}"
  SUM_URL="${BASE}/download/${VERSION}/SHA256SUMS"
fi

# --- download helper (curl or wget) ---
download() {
  local url="$1" out="$2"
  if command -v curl >/dev/null 2>&1; then
    curl -fSL "$url" -o "$out"
  elif command -v wget >/dev/null 2>&1; then
    wget -qO "$out" "$url"
  else
    echo "install.sh: requires curl or wget" >&2
    exit 1
  fi
}

# --- checksum helper: prints <hash> for a file ---
checksum() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | cut -d' ' -f1
  else
    shasum -a 256 "$1" | cut -d' ' -f1
  fi
}

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

echo "downloading ${ASSET}..." >&2
download "$BIN_URL" "$tmpdir/$ASSET"

echo "downloading SHA256SUMS..." >&2
download "$SUM_URL" "$tmpdir/SHA256SUMS"

# Verify: compute the binary's hash and compare to the expected entry.
expected=$(grep "  ${ASSET}\$" "$tmpdir/SHA256SUMS" | cut -d' ' -f1)
actual=$(checksum "$tmpdir/$ASSET")
if [ -z "$expected" ] || [ "$actual" != "$expected" ]; then
  echo "install.sh: checksum verification failed for ${ASSET}" >&2
  exit 1
fi

mv "$tmpdir/$ASSET" ./herdr-tab-rename
chmod +x ./herdr-tab-rename
echo "installed herdr-tab-rename (${OS}-${ARCH})" >&2
