# Lian Li UNI FAN USB HID protocol

This documents the reverse-engineered protocol `lianctl` implements. It is
synthesised from [`uni-sync`](https://github.com/EightB1ts/uni-sync) (fan
control) and [OpenRGB](https://gitlab.com/CalcProgrammer1/OpenRGB)'s
`LianLiUniHubSLInfinityController` (per-LED RGB), then verified byte-for-byte in
`internal/device/unihub_test.go`.

## Devices

Vendor ID: `0x0CF2`

| PID      | Model                | Fan curve (RPM)     | RGB per-LED |
|----------|----------------------|---------------------|-------------|
| `0x7750` | UNI FAN SL v1        | 800–1900            | not decoded |
| `0xA100` | UNI FAN SL v1        | 800–1900            | not decoded |
| `0xA101` | UNI FAN AL           | 800–1900            | not decoded |
| `0xA102` | UNI FAN **SL-Infinity** | 200–2100         | ✅ full      |
| `0xA103` | UNI FAN SL v2        | 250–2000            | not decoded |
| `0xA104` | UNI FAN AL v2        | 250–2000            | not decoded |
| `0xA105` | UNI FAN SL v2        | 250–2000            | not decoded |

The controller is a USB **composite** device. On Windows it shows interface
`MI_01` as an HID vendor-defined device; on Linux that surfaces as a
`/dev/hidrawN` node with VID `0cf2`. All commands below are HID **output
reports** whose first byte is the report ID `0xE0`.

## Fan control

There are 4 physical fan channels (`ch` = 0..3). Two registers, both addressed
through the "control" report at index 0x10:

### Channel mode

```
[0xE0, 0x10, MODE_REG, channel_byte]
channel_byte = (0x10 << ch) | (PWM ? (0x1 << ch) : 0)
```

`MODE_REG` is `0x31` (SL v1), `0x42` (AL), or `0x62` (SL-Infinity / v2). The
high nibble one-hot selects the channel; the low bit chooses motherboard-PWM
(`1`) vs manual (`0`).

### Manual speed

```
[0xE0, 0x20+ch, 0x00, speed]
```

`speed` is the RPM mapped into a single byte. For the SL-Infinity:

```
speed = (200 + 19 * percent) / 21        # integer division, percent in 0..100
```

(SL v1 / AL use `(800 + 11*p)/19`; v2 use `(250 + 17.5*p)/20`.)

## RGB — motherboard ARGB sync (all models)

```
[0xE0, 0x10, SYNC_REG, on, 0, 0, 0]      # on = 0 or 1
```

`SYNC_REG` is `0x30` (SL v1), `0x41` (AL), `0x61` (SL-Infinity / v2). When on,
the fans mirror the 5V-ARGB header instead of using stored colors/effects.

## RGB — per-LED (SL-Infinity, `0xA102`)

8 RGB channels (`ch` = 0..7), up to 96 LEDs per channel. Three packets:

### 1. Start / direct-action (65 bytes)

```
[0xE0, 0x10, 0x60, 1 + ch/2, 0x04, 0, 0, ...]
```

### 2. Color upload (353 bytes)

```
[0xE0, 0x30+ch, R0, B0, G0, R1, B1, G1, ...]
```

**Wire byte order is R, B, G** (not RGB). Up to 96 LEDs (288 color bytes).

### 3. Commit (65 bytes)

```
[0xE0, 0x10+ch, mode, speed, direction, brightness, 0, ...]
```

| Field      | Values |
|------------|--------|
| mode       | static `0x01`, breathing `0x02`, rainbow-morph `0x04`, rainbow `0x05`, staggered `0x18`, tide `0x1A`, runway `0x1C`, mixing `0x1E`, stack `0x20`, neon `0x22`, color-cycle `0x23`, meteor `0x24`, … (see `internal/device/effect.go`) |
| speed      | `0x02` slow … `0x00` mid … `0xFE` fast |
| direction  | LTR `0x00`, RTL `0x01` |
| brightness | 100% `0x00`, 75% `0x01`, 50% `0x02`, 25% `0x03`, off `0x08` |

For a static color: upload colors (step 2) then commit with `mode=0x01`.

## Not yet decoded

- **Per-LED RGB for non-SL-Infinity models.** The fan protocol is identical;
  only the RGB packet layout differs. Capture and add a driver — see
  [CAPTURE.md](CAPTURE.md).
- **LCD streaming.** Only the LCD-equipped devices (UNI FAN TL LCD / Universal
  Screen, separate PIDs) have a screen — the SL-Infinity does not. OpenRGB's
  `LianLiUniversalScreenController` is the reference if you add it.
- **RPM read-back.** L-Connect shows live RPM; the input-report format for that
  has not been confirmed here.
