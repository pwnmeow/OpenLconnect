// Package device contains the Lian Li controller drivers.
package device

import (
	"fmt"

	"github.com/lianctl/lianctl/internal/hid"
)

// VendorID is Lian Li's USB vendor ID.
const VendorID uint16 = 0x0CF2

// Model identifies a controller family. The byte-level protocol differs
// slightly between them (fan RPM curve, register bases, RGB layout).
type Model struct {
	PID  uint16
	Name string
}

// Known controllers. PIDs are from uni-sync / OpenRGB.
var Models = []Model{
	{0x7750, "UNI FAN SL v1"},
	{0xA100, "UNI FAN SL v1"},
	{0xA101, "UNI FAN AL"},
	{0xA102, "UNI FAN SL-Infinity"},
	{0xA103, "UNI FAN SL v2"},
	{0xA104, "UNI FAN AL v2"},
	{0xA105, "UNI FAN SL v2"},
}

func modelFor(pid uint16) (Model, bool) {
	for _, m := range Models {
		if m.PID == pid {
			return m, true
		}
	}
	return Model{}, false
}

// AllPIDs returns every supported product ID, for enumeration.
func AllPIDs() []uint16 {
	pids := make([]uint16, len(Models))
	for i, m := range Models {
		pids[i] = m.PID
	}
	return pids
}

// Color is a 24-bit RGB color.
type Color struct{ R, G, B uint8 }

// Controller is the capability surface a driver exposes. Not every model
// implements every method; unsupported features return ErrUnsupported.
type Controller interface {
	Info() hid.Info
	Model() Model

	// FanChannels is the number of physical fan ports.
	FanChannels() int
	// SetFanPercent sets a manual fan duty (0-100) on a channel.
	SetFanPercent(channel int, percent float64) error
	// SetFanPWM puts a channel under motherboard 4-pin PWM control.
	SetFanPWM(channel int) error

	// RGBChannels is the number of addressable RGB channels.
	RGBChannels() int
	// SetChannelColors uploads per-LED colors and applies them as a static
	// effect at the given brightness (0-100).
	SetChannelColors(channel int, colors []Color, brightness float64) error
	// SetChannelEffect runs a hardware effect on a channel.
	SetChannelEffect(channel int, e Effect) error
	// SetMotherboardSync toggles whether RGB mirrors the 5V ARGB header.
	SetMotherboardSync(on bool) error

	Close() error
}

// ErrUnsupported is returned for capabilities a model does not implement yet.
var ErrUnsupported = fmt.Errorf("operation not supported by this controller")

// OpenAll enumerates and opens every supported Lian Li controller present.
func OpenAll() ([]Controller, error) {
	infos, err := hid.Enumerate(VendorID, AllPIDs())
	if err != nil {
		return nil, err
	}
	var ctrls []Controller
	var firstErr error
	for _, info := range infos {
		c, err := openController(info)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		ctrls = append(ctrls, c)
	}
	if len(ctrls) == 0 && firstErr != nil {
		return nil, firstErr
	}
	return ctrls, nil
}

func openController(info hid.Info) (Controller, error) {
	m, ok := modelFor(info.ProductID)
	if !ok {
		return nil, fmt.Errorf("unknown PID %#04x", info.ProductID)
	}
	dev, err := hid.Open(info.Path)
	if err != nil {
		return nil, err
	}
	// All current models share the UniHub driver; per-model parameters are
	// selected from the PID inside it.
	return newUniHub(info, m, dev), nil
}
