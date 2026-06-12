#!/usr/bin/env bash
# Live hardware test for lianctl inside WSL, after the SL-Infinity hub has been
# attached via `usbipd attach --wsl`. hidraw nodes are root-owned in WSL (no
# udev), so we run the binary via sudo. Uses sudo -n; if that needs a password
# the script tells you instead of hanging.
set -u

BIN=/tmp/lianctl
cp /mnt/c/Users/sheer/lianctl/build/lianctl "$BIN" && chmod +x "$BIN"

# Pick a runner that can read /dev/hidraw (root). Try direct, then sudo -n.
RUN=("$BIN")
if [ ! -r /dev/hidraw0 ] 2>/dev/null; then
  if sudo -n true 2>/dev/null; then
    RUN=(sudo "$BIN")
  fi
fi
echo "runner: ${RUN[*]}"

echo "================ presence ================"
echo "-- /sys/class/hidraw --"; ls /sys/class/hidraw 2>&1
echo "-- /dev/hidraw* --";      ls -l /dev/hidraw* 2>&1
echo "-- usbip vid:pid (sysfs) --"
grep -rl . /sys/bus/usb/devices/*/idVendor 2>/dev/null | while read -r f; do
  d=$(dirname "$f"); v=$(cat "$d/idVendor"); p=$(cat "$d/idProduct" 2>/dev/null)
  [ "$v" = "0cf2" ] && echo "  found 0cf2:$p at $d"
done

echo "================ lianctl list ================"
"${RUN[@]}" list; echo "list exit=$?"

echo "================ RGB test (safe, visible) ================"
echo ">> ch0 -> GREEN #00ff88"; "${RUN[@]}" color 0 '#00ff88' 100; echo "  exit=$?"; sleep 2
echo ">> ch0 -> PURPLE #5a00ff"; "${RUN[@]}" color 0 '#5a00ff' 100; echo "  exit=$?"; sleep 2
echo ">> ch0 -> RAINBOW effect"; "${RUN[@]}" effect 0 rainbow speed=80; echo "  exit=$?"

echo "================ FAN test ================"
echo ">> ch0 -> 50%";  "${RUN[@]}" fan 0 50;  echo "  exit=$?"; sleep 3
echo ">> ch0 -> 100%"; "${RUN[@]}" fan 0 100; echo "  exit=$?"; sleep 3
echo ">> ch0 -> 40%";  "${RUN[@]}" fan 0 40;  echo "  exit=$?"

echo "================ done ================"
