#!/usr/bin/env bash
# alice-installer bootstrap script.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/Jcaltamar/alice-installer/main/scripts/install.sh | bash
#
# Environment overrides:
#   INSTALL_DIR   destination for the binary (default: $HOME/.local/bin)
#   VERSION       specific release tag (default: latest)
#   REPO          override repo slug (default: Jcaltamar/alice-installer)

set -euo pipefail

REPO="${REPO:-Jcaltamar/alice-installer}"
BINARY="alice-installer"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"
VERSION="${VERSION:-}"

c_reset=$'\033[0m'
c_cyan=$'\033[36m'
c_green=$'\033[32m'
c_red=$'\033[31m'
c_yellow=$'\033[33m'

info()  { printf '%s→%s %s\n' "$c_cyan" "$c_reset" "$*"; }
ok()    { printf '%s✓%s %s\n' "$c_green" "$c_reset" "$*"; }
warn()  { printf '%s!%s %s\n' "$c_yellow" "$c_reset" "$*" >&2; }
fail()  { printf '%s✗%s %s\n' "$c_red" "$c_reset" "$*" >&2; exit 1; }

require() {
  command -v "$1" >/dev/null 2>&1 || fail "Missing required tool: $1"
}

require curl
require tar
require uname

if command -v sha256sum >/dev/null 2>&1; then
  SHA="sha256sum"
elif command -v shasum >/dev/null 2>&1; then
  SHA="shasum -a 256"
else
  fail "Need sha256sum or shasum for checksum verification"
fi

os=$(uname -s | tr '[:upper:]' '[:lower:]')
if [ "$os" != "linux" ]; then
  fail "Only Linux is supported in v1 (detected: $os). macOS and Windows support is planned."
fi

raw_arch=$(uname -m)
case "$raw_arch" in
  x86_64|amd64) arch="amd64" ;;
  aarch64|arm64) arch="arm64" ;;
  *) fail "Unsupported CPU architecture: $raw_arch (supported: amd64, arm64)" ;;
esac

if [ -z "$VERSION" ]; then
  info "Looking up latest release of $REPO…"
  VERSION=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" \
    | sed -n 's/.*"tag_name":[[:space:]]*"\([^"]*\)".*/\1/p' \
    | head -n1)
  [ -n "$VERSION" ] || fail "Could not determine latest release from GitHub API"
fi

version_no_v="${VERSION#v}"
archive="${BINARY}_${version_no_v}_${os}_${arch}.tar.gz"
base_url="https://github.com/$REPO/releases/download/$VERSION"
url="$base_url/$archive"
checksum_url="$base_url/checksums.txt"

tmpdir=$(mktemp -d)
trap 'rm -rf "$tmpdir"' EXIT

info "Downloading $archive"
curl -fsSL "$url" -o "$tmpdir/$archive" \
  || fail "Failed to download $url — check that the release exists"

info "Verifying SHA256 checksum"
curl -fsSL "$checksum_url" -o "$tmpdir/checksums.txt" \
  || fail "Failed to download checksums.txt"

expected=$(awk -v a="$archive" '$2 == a { print $1; exit }' "$tmpdir/checksums.txt")
[ -n "$expected" ] || fail "No checksum entry for $archive"

actual=$($SHA "$tmpdir/$archive" | awk '{print $1}')
if [ "$expected" != "$actual" ]; then
  fail "Checksum mismatch for $archive
  expected: $expected
  actual:   $actual"
fi
ok "Checksum matches ($expected)"

info "Extracting"
tar -xzf "$tmpdir/$archive" -C "$tmpdir"
[ -x "$tmpdir/$BINARY" ] || fail "Binary missing or not executable after extract"

info "Installing to $INSTALL_DIR"
mkdir -p "$INSTALL_DIR"
install -m 0755 "$tmpdir/$BINARY" "$INSTALL_DIR/$BINARY"

ok "Installed $BINARY $VERSION → $INSTALL_DIR/$BINARY"

case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *)
    warn "$INSTALL_DIR is not in your PATH. Add this to your shell config:"
    printf '      export PATH="%s:$PATH"\n' "$INSTALL_DIR"
    ;;
esac

printf '\nNext: run %s%s%s\n' "$c_cyan" "$BINARY" "$c_reset"
