#!/bin/sh
set -e
REPO="V12KLT/authris-cli"
DEST="${AUTHRIS_DEST:-$HOME/.local/bin}"

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$os" in
  linux|darwin) ;;
  *) echo "unsupported os: $os" >&2; exit 1 ;;
esac

arch="$(uname -m)"
case "$arch" in
  x86_64|amd64) arch="amd64" ;;
  aarch64|arm64) arch="arm64" ;;
  *) echo "unsupported arch: $arch" >&2; exit 1 ;;
esac

fetch() {
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$1"
  elif command -v wget >/dev/null 2>&1; then
    wget -qO- "$1"
  else
    echo "need curl or wget" >&2
    exit 1
  fi
}

tag="${AUTHRIS_VERSION:-}"
if [ -z "$tag" ]; then
  tag="$(fetch "https://api.github.com/repos/$REPO/releases/latest" | grep -m1 '"tag_name"' | cut -d'"' -f4)"
fi
if [ -z "$tag" ]; then
  echo "could not resolve latest release" >&2
  exit 1
fi

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
fetch "https://github.com/$REPO/releases/download/$tag/authris-$os-$arch" > "$tmp/authris"
chmod +x "$tmp/authris"
mkdir -p "$DEST"
mv "$tmp/authris" "$DEST/authris"

case ":$PATH:" in
  *":$DEST:"*) ;;
  *) echo "add to PATH: export PATH=\"$DEST:\$PATH\"" ;;
esac
echo "installed authris $tag to $DEST/authris"
"$DEST/authris" 2>&1 | head -2 || true
