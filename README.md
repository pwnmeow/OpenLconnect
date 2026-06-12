# OpenLConnect

**An open-source Linux replacement for Lian Li L-Connect 3.** Control UNI FAN
controller **fan speeds** (manual % and temperature curves) and **ARGB**
(per-LED colors + hardware effects) directly over USB HID — no vendor software,
no cloud, no GUI required. The command-line tool is `lianctl`.

Pure Go, **zero external dependencies**, no cgo, no libusb. Talks straight to
`/dev/hidraw*`.

> Status: fan control works on **all** UNI FAN controllers. Full per-LED RGB is
> implemented and **verified on real hardware** for the **UNI FAN SL-Infinity**
> (`0cf2:a102`). RGB for other models and LCD streaming are framework-ready —
> see [`docs/PROTOCOL.md`](docs/PROTOCOL.md) and [`docs/CAPTURE.md`](docs/CAPTURE.md)
> to help decode them.
>
> _Not affiliated with or endorsed by Lian Li. "L-Connect" is used only to
> describe compatibility._

## Supported hardware

| Controller | Fan | RGB |
|---|---|---|
| UNI FAN SL v1 (`0cf2:7750`, `a100`) | ✅ | sync only |
| UNI FAN AL (`a101`) | ✅ | sync only |
| **UNI FAN SL-Infinity (`a102`)** | ✅ | ✅ per-LED + effects |
| UNI FAN SL v2 (`a103`, `a105`) | ✅ | sync only |
| UNI FAN AL v2 (`a104`) | ✅ | sync only |

## Install

Requires Go 1.25+ on the Linux machine (or cross-compile — see below).

```bash
git clone https://github.com/pwnmeow/OpenLconnect
cd OpenLconnect
./scripts/install.sh            # builds, installs binary + udev rule
# optional: also install & enable the fan-curve daemon
./scripts/install.sh --service
```

Re-login or replug the controller once so the udev rule grants access. Then:

```bash
lianctl list
```

### Cross-compile from another OS

```bash
GOOS=linux GOARCH=amd64 go build -o lianctl ./cmd/lianctl   # x86-64
GOOS=linux GOARCH=arm64 go build -o lianctl ./cmd/lianctl   # ARM (e.g. SBCs)
```

## Usage

```bash
lianctl list                      # detected controllers
lianctl sensors                   # available temperature sources

# Fans (channels 0-3)
lianctl fan 0 60                  # set channel 0 to 60%
lianctl fan 1 pwm                 # hand channel 1 to motherboard 4-pin PWM

# RGB (SL-Infinity, channels 0-7)
lianctl color 0 '#5a00ff' 100     # solid purple, 100% brightness
lianctl effect 0 rainbow speed=80 # hardware rainbow
lianctl effect 0 breathing color='#00ff88' bri=50
lianctl effects                   # list effect names
lianctl sync on                   # mirror the motherboard 5V ARGB header

# Daemon (temperature-driven fan curves)
lianctl config init               # write a starter config
lianctl daemon                    # run curves in the foreground
```

## Fan curves

`lianctl config init` writes JSON to `~/.config/lianctl/config.json` (or
`/etc/lianctl/config.json` for the system service). Example:

```json
{
  "poll_seconds": 2,
  "fans": [
    {
      "channel": 0,
      "source": "hwmon:k10temp/Tctl",
      "curve": [[30, 30], [50, 45], [65, 70], [80, 100]]
    },
    { "channel": 1, "source": "max", "curve": [[40, 40], [70, 100]] }
  ],
  "rgb": [
    { "channel": 0, "effect": "static", "color": "#5a00ff", "brightness": 100 }
  ]
}
```

`curve` points are `[tempC, dutyPercent]`, linearly interpolated and clamped at
the ends. Temperature `source` can be:

- `max` — hottest hwmon sensor on the system
- `hwmon:<chip>/<label>` — e.g. `hwmon:k10temp/Tctl`, `hwmon:coretemp/Package id 0`
- `file:/sys/class/hwmon/hwmon2/temp1_input` — a raw sysfs millidegree file
- `cmd:<shell command>` — any command printing degrees C on stdout

Run `lianctl sensors` to see what's available on your box.

## How it works

Each command is a HID output report (report ID `0xE0`) written to the
controller's hidraw node. The full byte-level protocol — fan RPM mapping,
channel-mode register, and the SL-Infinity RGB packets — is documented in
[`docs/PROTOCOL.md`](docs/PROTOCOL.md) and asserted in
`internal/device/unihub_test.go`.

```
cmd/lianctl        CLI
internal/device    controller drivers (uniHub) + protocol
internal/hid       /dev/hidraw transport (pure Go, Linux)
internal/sensors   temperature sources
internal/config    JSON config + curve interpolation
internal/daemon    curve evaluation loop
```

## Contributing

Highest-value next steps:

1. **Per-LED RGB for SL/AL/v2 hubs** — capture L-Connect traffic
   ([`docs/CAPTURE.md`](docs/CAPTURE.md)) and add a driver case.
2. **LCD streaming** for the TL-LCD / Universal-Screen devices.
3. **RPM read-back** so the daemon can log/limit by actual fan RPM.

PRs welcome. Add a byte-level test for any new packet.

## Credits

Protocol knowledge stands on the shoulders of
[`uni-sync`](https://github.com/EightB1ts/uni-sync) (fan control) and
[OpenRGB](https://gitlab.com/CalcProgrammer1/OpenRGB) (RGB). This project is an
independent clean-room Go implementation for Linux.

## License

MIT — see [LICENSE](LICENSE). Not affiliated with or endorsed by Lian Li.
