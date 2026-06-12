// Package config loads/saves the daemon configuration (JSON, no deps).
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// Config is the on-disk daemon configuration.
type Config struct {
	// PollSeconds is how often fan curves are evaluated.
	PollSeconds float64 `json:"poll_seconds"`
	// Fans maps "channel" -> curve. Channels are 0-based fan ports.
	Fans []FanRule `json:"fans"`
	// RGB applies a startup lighting setup (optional).
	RGB []RGBRule `json:"rgb,omitempty"`
}

// FanRule is a temperature->duty curve for one fan channel.
type FanRule struct {
	Channel int    `json:"channel"`
	Source  string `json:"source"` // see sensors.Read
	// Curve is a list of [tempC, dutyPercent] points, ascending by temp.
	// Linearly interpolated; clamped to the ends.
	Curve [][2]float64 `json:"curve"`
	// PWM, if true, hands the channel to motherboard PWM and ignores Curve.
	PWM bool `json:"pwm,omitempty"`
}

// RGBRule sets a channel to an effect or solid color at startup.
type RGBRule struct {
	Channel    int     `json:"channel"`
	Effect     string  `json:"effect"`               // effect name, e.g. "static", "rainbow"
	Color      string  `json:"color,omitempty"`      // hex "#RRGGBB" for static/color effects
	Brightness float64 `json:"brightness,omitempty"` // 0-100
	Speed      float64 `json:"speed,omitempty"`      // 0-100
}

// Default returns a sensible starter config.
func Default() Config {
	return Config{
		PollSeconds: 2,
		Fans: []FanRule{{
			Channel: 0,
			Source:  "max",
			Curve: [][2]float64{
				{30, 30}, {45, 40}, {60, 60}, {75, 100},
			},
		}},
		RGB: []RGBRule{{
			Channel: 0, Effect: "static", Color: "#5a00ff", Brightness: 100,
		}},
	}
}

// Interp returns the interpolated duty for a temperature on this rule's curve.
func (r FanRule) Interp(temp float64) float64 {
	pts := r.Curve
	if len(pts) == 0 {
		return 0
	}
	if temp <= pts[0][0] {
		return pts[0][1]
	}
	last := pts[len(pts)-1]
	if temp >= last[0] {
		return last[1]
	}
	for i := 1; i < len(pts); i++ {
		if temp <= pts[i][0] {
			t0, d0 := pts[i-1][0], pts[i-1][1]
			t1, d1 := pts[i][0], pts[i][1]
			if t1 == t0 {
				return d1
			}
			return d0 + (d1-d0)*(temp-t0)/(t1-t0)
		}
	}
	return last[1]
}

// Load reads a config from path.
func Load(path string) (Config, error) {
	var c Config
	b, err := os.ReadFile(path)
	if err != nil {
		return c, err
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return c, fmt.Errorf("parsing %s: %w", path, err)
	}
	// Keep curves sorted so interpolation is well-defined.
	for i := range c.Fans {
		sort.Slice(c.Fans[i].Curve, func(a, b int) bool {
			return c.Fans[i].Curve[a][0] < c.Fans[i].Curve[b][0]
		})
	}
	if c.PollSeconds <= 0 {
		c.PollSeconds = 2
	}
	return c, nil
}

// Save writes config to path (pretty JSON), creating parent dirs.
func Save(path string, c Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

// DefaultPath returns the standard config location.
func DefaultPath() string {
	if x := os.Getenv("LIANCTL_CONFIG"); x != "" {
		return x
	}
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "lianctl", "config.json")
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".config", "lianctl", "config.json")
	}
	return "/etc/lianctl/config.json"
}
