package device

import (
	"fmt"
	"strings"
)

// Direction of a moving effect.
type Direction uint8

const (
	DirLTR Direction = iota // left-to-right
	DirRTL                  // right-to-left
)

// Effect describes a hardware lighting effect on a channel.
type Effect struct {
	Mode       string  // effect name, see EffectModes
	Speed      float64 // 0-100
	Brightness float64 // 0-100
	Direction  Direction
	Colors     []Color // colors used by the effect, if any
}

// EffectModes maps friendly names to SL-Infinity mode bytes.
// Names match L-Connect 3 where practical.
var EffectModes = map[string]uint8{
	"static":        0x01,
	"breathing":     0x02,
	"rainbow-morph": 0x04,
	"rainbow":       0x05,
	"staggered":     0x18,
	"tide":          0x1A,
	"runway":        0x1C,
	"mixing":        0x1E,
	"stack":         0x20,
	"stack-multi":   0x21,
	"neon":          0x22,
	"color-cycle":   0x23,
	"meteor":        0x24,
	"voice":         0x26,
	"groove":        0x27,
	"render":        0x28,
	"tunnel":        0x29,
}

// ParseHexColor parses "#RRGGBB" or "RRGGBB" into a Color.
func ParseHexColor(s string) (Color, error) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) != 6 {
		return Color{}, fmt.Errorf("color %q must be 6 hex digits (#RRGGBB)", s)
	}
	var c Color
	n, err := fmt.Sscanf(s, "%02x%02x%02x", &c.R, &c.G, &c.B)
	if err != nil || n != 3 {
		return Color{}, fmt.Errorf("invalid hex color %q", s)
	}
	return c, nil
}

// EffectModeNames returns the sorted-ish list of supported effect names.
func EffectModeNames() []string {
	names := make([]string, 0, len(EffectModes))
	for n := range EffectModes {
		names = append(names, n)
	}
	return names
}

func modeByte(name string) (uint8, error) {
	if b, ok := EffectModes[strings.ToLower(name)]; ok {
		return b, nil
	}
	return 0, fmt.Errorf("unknown effect %q (try one of: %s)", name, strings.Join(EffectModeNames(), ", "))
}

// brightnessByte maps a 0-100 brightness to the nearest SL-Infinity code.
func brightnessByte(pct float64) uint8 {
	switch {
	case pct <= 12:
		return 0x08 // off-ish / 0%
	case pct <= 37:
		return 0x03 // 25%
	case pct <= 62:
		return 0x02 // 50%
	case pct <= 87:
		return 0x01 // 75%
	default:
		return 0x00 // 100%
	}
}

// speedByte maps a 0-100 effect speed to the nearest SL-Infinity code.
func speedByte(pct float64) uint8 {
	switch {
	case pct <= 12:
		return 0x02 // slowest
	case pct <= 37:
		return 0x01
	case pct <= 62:
		return 0x00
	case pct <= 87:
		return 0xFF
	default:
		return 0xFE // fastest
	}
}
