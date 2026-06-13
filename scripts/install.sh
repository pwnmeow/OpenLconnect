#!/usr/bin/env bash
# Install lianctl + udev rule + (optional) systemd service.
#
# By default this downloads a prebuilt binary from the latest GitHub release,
# so you do NOT need Go installed. Set BUILD_FROM_SOURCE=1 (or just have Go on
# your PATH with no network) to build from the local checkout instead.
set -euo pipefail

cd "$(dirname "$0")/.."

REPO="pwnmeow/OpenLconnect"
BIN="lianctl"

# --- detect CPU arch ---------------------------------------------------------
case "$(uname -m)" in
  x86_64|amd64)  ARCH=amd64 ;;
  aarch64|arm64) ARCH=arm64 ;;
  *) echo "error: unsupported CPU arch '$(uname -m)' (only amd64/arm64 prebuilt)" >&2; exit 1 ;;
esac

# --- tiny http helper (curl or wget) ----------------------------------------
fetch() { # fetch <url> <outfile>   (outfile "-" => stdout)
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$1" -o "$2"
  elif command -v wget >/dev/null 2>&1; then
    wget -qO "$2" "$1"
  else
    echo "error: need 'curl' or 'wget' to download the release" >&2; exit 1
  fi
}

mkdir -p build

build_from_source() {
  command -v go >/dev/null 2>&1 || return 1
  echo "==> building from source (go found)"
  go build -o "build/$BIN" ./cmd/lianctl
}

download_prebuilt() {
  command -v curl >/dev/null 2>&1 || command -v wget >/dev/null 2>&1 || return 1
  echo "==> finding latest release"
  local tag
  tag="$(fetch "https://api.github.com/repos/$REPO/releases/latest" - \
        | grep -m1 '"tag_name"' \
        | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/')"
  [ -n "$tag" ] || { echo "   could not determine latest release tag" >&2; return 1; }

  local url tmp
  url="https://github.com/$REPO/releases/download/$tag/${BIN}-${tag}-linux-${ARCH}.tar.gz"
  tmp="$(mktemp -d)"
  trap 'rm -rf "$tmp"' RETURN
  echo "==> downloading $BIN $tag ($ARCH)"
  fetch "$url" "$tmp/pkg.tar.gz" || { echo "   download failed: $url" >&2; return 1; }
  tar -xzf "$tmp/pkg.tar.gz" -C "$tmp"
  install -m0755 "$tmp/$BIN" "build/$BIN"
}

# --- get the binary into build/ ---------------------------------------------
# Prefer prebuilt (no toolchain needed). Force source with BUILD_FROM_SOURCE=1.
if [ "${BUILD_FROM_SOURCE:-0}" = "1" ]; then
  build_from_source || { echo "error: BUILD_FROM_SOURCE=1 but Go is not installed (need Go 1.25+). See https://go.dev/dl/" >&2; exit 1; }
elif ! download_prebuilt; then
  echo "==> prebuilt download unavailable, trying to build from source"
  build_from_source || {
    echo "error: could not download a prebuilt binary and Go is not installed." >&2
    echo "       Install curl/wget for the prebuilt path, or Go 1.25+ to build." >&2
    echo "       See https://go.dev/dl/" >&2
    exit 1
  }
fi

echo "==> installing binary to /usr/local/bin (sudo)"
sudo install -m0755 "build/$BIN" "/usr/local/bin/$BIN"

echo "==> installing udev rule (sudo)"
sudo install -m0644 packaging/udev/99-lianli.rules /etc/udev/rules.d/99-lianli.rules
sudo udevadm control --reload
sudo udevadm trigger

echo "==> creating default config at /etc/lianctl/config.json (if missing)"
if [ ! -f /etc/lianctl/config.json ]; then
  sudo "/usr/local/bin/$BIN" config init --config /etc/lianctl/config.json
fi

if [ "${1:-}" = "--service" ]; then
  echo "==> installing + enabling systemd service (sudo)"
  sudo install -m0644 packaging/systemd/lianctl.service /etc/systemd/system/lianctl.service
  sudo systemctl daemon-reload
  sudo systemctl enable --now lianctl.service
  echo "service status: systemctl status lianctl"
fi

echo "==> done. Try:  lianctl list"
echo "    (you may need to unplug/replug the controller once, or re-login, for udev to take effect)"
