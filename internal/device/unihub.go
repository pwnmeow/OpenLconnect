package device

import (
	"fmt"

	"github.com/pwnmeow/OpenLconnect/internal/hid"
)

// Report ID prefixing every Lian Li UNI Hub command.
const transactionID = 0xE0

// uniHub drives every current UNI FAN controller. Per-model differences (fan
// RPM curve, RGB sync/mode registers, RGB packet layout) are selected from the
// PID via modelParams.
type uniHub struct {
	info   hid.Info
	model  Model
	dev    hid.Device
	params modelParams
}

type modelParams struct {
	// fanSpeed converts a 0-100 duty into the controller's RPM byte.
	fanSpeed func(pct float64) byte
	// rgbSyncReg / chanModeReg are the register bytes for the motherboard
	// RGB-sync toggle and the per-channel fan-mode write.
	rgbSyncReg  byte
	chanModeReg byte
	// fanChannels physical fan ports.
	fanChannels int
	// slInfinityRGB selects the 8-channel SL-Infinity per-LED RGB protocol.
	slInfinityRGB bool
	rgbChannels   int
}

func paramsFor(pid uint16) modelParams {
	// Fan RPM curves, transcribed from uni-sync.
	sl1 := func(p float64) byte { return byte(int(800.0+11.0*p) / 19) }   // SL v1 / AL
	sl2 := func(p float64) byte { return byte(int(250.0+17.5*p) / 20) }   // SL v2 / AL v2
	slInf := func(p float64) byte { return byte(int(200.0+19.0*p) / 21) } // SL-Infinity

	switch pid {
	case 0x7750, 0xA100: // SL v1
		return modelParams{fanSpeed: sl1, rgbSyncReg: 0x30, chanModeReg: 0x31, fanChannels: 4, rgbChannels: 4}
	case 0xA101: // AL
		return modelParams{fanSpeed: sl1, rgbSyncReg: 0x41, chanModeReg: 0x42, fanChannels: 4, rgbChannels: 4}
	case 0xA102: // SL-Infinity
		return modelParams{fanSpeed: slInf, rgbSyncReg: 0x61, chanModeReg: 0x62, fanChannels: 4, slInfinityRGB: true, rgbChannels: 8}
	case 0xA103, 0xA105: // SL v2
		return modelParams{fanSpeed: sl2, rgbSyncReg: 0x61, chanModeReg: 0x62, fanChannels: 4, rgbChannels: 4}
	case 0xA104: // AL v2
		return modelParams{fanSpeed: sl2, rgbSyncReg: 0x61, chanModeReg: 0x62, fanChannels: 4, rgbChannels: 4}
	default:
		return modelParams{fanSpeed: sl1, rgbSyncReg: 0x30, chanModeReg: 0x31, fanChannels: 4, rgbChannels: 4}
	}
}

func newUniHub(info hid.Info, m Model, dev hid.Device) *uniHub {
	return &uniHub{info: info, model: m, dev: dev, params: paramsFor(m.PID)}
}

func (u *uniHub) Info() hid.Info   { return u.info }
func (u *uniHub) Model() Model     { return u.model }
func (u *uniHub) FanChannels() int { return u.params.fanChannels }
func (u *uniHub) RGBChannels() int { return u.params.rgbChannels }
func (u *uniHub) Close() error     { return u.dev.Close() }

func (u *uniHub) checkFanChannel(ch int) error {
	if ch < 0 || ch >= u.params.fanChannels {
		return fmt.Errorf("fan channel %d out of range (0-%d)", ch, u.params.fanChannels-1)
	}
	return nil
}

// SetFanPercent puts a channel in manual mode and sets its duty.
func (u *uniHub) SetFanPercent(channel int, percent float64) error {
	if err := u.checkFanChannel(channel); err != nil {
		return err
	}
	percent = clamp(percent, 0, 100)

	// Channel mode: manual (PWM bit clear). High nibble one-hot selects the
	// channel: channel_byte = (0x10 << ch).
	channelByte := byte(0x10 << uint(channel))
	if _, err := u.dev.Write([]byte{transactionID, 0x10, u.params.chanModeReg, channelByte}); err != nil {
		return fmt.Errorf("set manual mode ch%d: %w", channel, err)
	}

	speed := u.params.fanSpeed(percent)
	if _, err := u.dev.Write([]byte{transactionID, byte(0x20 + channel), 0x00, speed}); err != nil {
		return fmt.Errorf("set speed ch%d: %w", channel, err)
	}
	return nil
}

// SetFanPWM hands a channel to the motherboard 4-pin PWM signal.
func (u *uniHub) SetFanPWM(channel int) error {
	if err := u.checkFanChannel(channel); err != nil {
		return err
	}
	channelByte := byte(0x10<<uint(channel)) | byte(0x1<<uint(channel))
	if _, err := u.dev.Write([]byte{transactionID, 0x10, u.params.chanModeReg, channelByte}); err != nil {
		return fmt.Errorf("set PWM mode ch%d: %w", channel, err)
	}
	return nil
}

// SetMotherboardSync toggles RGB mirroring of the 5V ARGB header.
func (u *uniHub) SetMotherboardSync(on bool) error {
	var sync byte
	if on {
		sync = 1
	}
	_, err := u.dev.Write([]byte{transactionID, 0x10, u.params.rgbSyncReg, sync, 0, 0, 0})
	return err
}

// ---- RGB (SL-Infinity per-LED) ----

const (
	slinfColorPacketLen = 353 // 0xE0, 0x30+ch, then up to 117 LEDs * 3
	slinfActionLen      = 65
	slinfMaxLEDs        = 96
)

func (u *uniHub) checkRGBChannel(ch int) error {
	if ch < 0 || ch >= u.params.rgbChannels {
		return fmt.Errorf("RGB channel %d out of range (0-%d)", ch, u.params.rgbChannels-1)
	}
	return nil
}

// SetChannelColors uploads per-LED colors and commits them as a static effect.
func (u *uniHub) SetChannelColors(channel int, colors []Color, brightness float64) error {
	if !u.params.slInfinityRGB {
		return fmt.Errorf("%w: per-LED RGB for %s is not decoded yet (see docs/PROTOCOL.md)", ErrUnsupported, u.model.Name)
	}
	if err := u.checkRGBChannel(channel); err != nil {
		return err
	}
	if len(colors) > slinfMaxLEDs {
		colors = colors[:slinfMaxLEDs]
	}

	if err := u.slinfStart(channel); err != nil {
		return err
	}
	if err := u.slinfUploadColors(channel, colors); err != nil {
		return err
	}
	return u.slinfCommit(channel, Effect{
		Mode:       "static",
		Brightness: brightness,
		Speed:      50,
		Direction:  DirLTR,
	})
}

// SetChannelEffect runs a hardware effect; if e.Colors is set they are uploaded
// first so color-aware effects use them.
func (u *uniHub) SetChannelEffect(channel int, e Effect) error {
	if !u.params.slInfinityRGB {
		return fmt.Errorf("%w: effects for %s are not decoded yet", ErrUnsupported, u.model.Name)
	}
	if err := u.checkRGBChannel(channel); err != nil {
		return err
	}
	if err := u.slinfStart(channel); err != nil {
		return err
	}
	if len(e.Colors) > 0 {
		if err := u.slinfUploadColors(channel, e.Colors); err != nil {
			return err
		}
	}
	return u.slinfCommit(channel, e)
}

// slinfStart sends the "begin direct action" packet for a channel.
func (u *uniHub) slinfStart(channel int) error {
	buf := make([]byte, slinfActionLen)
	buf[0] = transactionID
	buf[1] = 0x10
	buf[2] = 0x60
	buf[3] = byte(1 + channel/2) // fan-array selector
	buf[4] = 0x04                // number of fans driven
	_, err := u.dev.Write(buf)
	return err
}

// slinfUploadColors writes the per-LED color buffer. Wire order is R,B,G.
func (u *uniHub) slinfUploadColors(channel int, colors []Color) error {
	buf := make([]byte, slinfColorPacketLen)
	buf[0] = transactionID
	buf[1] = byte(0x30 + channel)
	off := 2
	for _, c := range colors {
		buf[off+0] = c.R
		buf[off+1] = c.B
		buf[off+2] = c.G
		off += 3
	}
	_, err := u.dev.Write(buf)
	return err
}

// slinfCommit applies an effect/brightness to a channel.
func (u *uniHub) slinfCommit(channel int, e Effect) error {
	mode, err := modeByte(e.Mode)
	if err != nil {
		return err
	}
	buf := make([]byte, slinfActionLen)
	buf[0] = transactionID
	buf[1] = byte(0x10 + channel)
	buf[2] = mode
	buf[3] = speedByte(e.Speed)
	buf[4] = byte(e.Direction)
	buf[5] = brightnessByte(e.Brightness)
	_, err = u.dev.Write(buf)
	return err
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
