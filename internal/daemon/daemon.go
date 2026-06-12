// Package daemon runs fan curves continuously against the open controllers.
package daemon

import (
	"context"
	"log"
	"time"

	"github.com/pwnmeow/OpenLconnect/internal/config"
	"github.com/pwnmeow/OpenLconnect/internal/device"
	"github.com/pwnmeow/OpenLconnect/internal/sensors"
)

// Run applies RGB rules once, then evaluates fan curves on PollSeconds until
// ctx is cancelled. Controllers are owned by the caller.
func Run(ctx context.Context, ctrls []device.Controller, cfg config.Config) error {
	applyRGB(ctrls, cfg)

	interval := time.Duration(cfg.PollSeconds * float64(time.Second))
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	log.Printf("lianctl daemon: %d controller(s), polling every %s", len(ctrls), interval)
	tick(ctrls, cfg) // apply immediately

	for {
		select {
		case <-ctx.Done():
			log.Printf("lianctl daemon: shutting down")
			return ctx.Err()
		case <-ticker.C:
			tick(ctrls, cfg)
		}
	}
}

func tick(ctrls []device.Controller, cfg config.Config) {
	for _, rule := range cfg.Fans {
		if rule.PWM {
			for _, c := range ctrls {
				if rule.Channel < c.FanChannels() {
					_ = c.SetFanPWM(rule.Channel)
				}
			}
			continue
		}
		temp, err := sensors.Read(rule.Source)
		if err != nil {
			log.Printf("ch%d: temp source %q: %v", rule.Channel, rule.Source, err)
			continue
		}
		duty := rule.Interp(temp)
		for _, c := range ctrls {
			if rule.Channel >= c.FanChannels() {
				continue
			}
			if err := c.SetFanPercent(rule.Channel, duty); err != nil {
				log.Printf("ch%d: set %.0f%%: %v", rule.Channel, duty, err)
			}
		}
	}
}

func applyRGB(ctrls []device.Controller, cfg config.Config) {
	for _, r := range cfg.RGB {
		eff := device.Effect{
			Mode:       r.Effect,
			Brightness: r.Brightness,
			Speed:      r.Speed,
		}
		if r.Color != "" {
			col, err := device.ParseHexColor(r.Color)
			if err != nil {
				log.Printf("rgb ch%d: %v", r.Channel, err)
				continue
			}
			// Fill the whole channel with the solid color.
			eff.Colors = fill(col, 96)
		}
		for _, c := range ctrls {
			if r.Channel >= c.RGBChannels() {
				continue
			}
			if err := c.SetChannelEffect(r.Channel, eff); err != nil {
				log.Printf("rgb ch%d: %v", r.Channel, err)
			}
		}
	}
}

func fill(c device.Color, n int) []device.Color {
	out := make([]device.Color, n)
	for i := range out {
		out[i] = c
	}
	return out
}
