#!/usr/bin/env bash
# Build and install lianctl + udev rule + (optional) systemd service.
set -euo pipefail

cd "$(dirname "$0")/.."

echo "==> building"
go build -o build/lianctl ./cmd/lianctl

echo "==> installing binary to /usr/local/bin (sudo)"
sudo install -m0755 build/lianctl /usr/local/bin/lianctl

echo "==> installing udev rule (sudo)"
sudo install -m0644 packaging/udev/99-lianli.rules /etc/udev/rules.d/99-lianli.rules
sudo udevadm control --reload
sudo udevadm trigger

echo "==> creating default config at /etc/lianctl/config.json (if missing)"
if [ ! -f /etc/lianctl/config.json ]; then
  sudo LIANCTL_CONFIG=/etc/lianctl/config.json /usr/local/bin/lianctl config init --config /etc/lianctl/config.json
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
