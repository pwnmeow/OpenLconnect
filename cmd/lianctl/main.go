// Command lianctl is an open-source Linux replacement for Lian Li L-Connect 3,
// controlling UNI FAN controllers (fan speed + ARGB) over USB HID.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/pwnmeow/OpenLconnect/internal/config"
	"github.com/pwnmeow/OpenLconnect/internal/daemon"
	"github.com/pwnmeow/OpenLconnect/internal/device"
	"github.com/pwnmeow/OpenLconnect/internal/sensors"
)

const usage = `lianctl - open-source Linux control for Lian Li UNI FAN controllers

USAGE:
  lianctl list                          list detected controllers
  lianctl sensors                       list available temperature sources
  lianctl temp                          show temperatures, color-coded by heat
  lianctl fan <ch> <percent>            set a fan channel to a manual duty (0-100)
  lianctl fan <ch> pwm                  hand a fan channel to motherboard PWM
  lianctl speed <low|medium|high|full>  set all fan channels to a preset duty
  lianctl color <ch> <#RRGGBB> [bri]    solid color on an RGB channel (bri 0-100)
  lianctl rgb off                       turn off all RGB on every channel
  lianctl rgb on [#RRGGBB] [bri]        turn all RGB on (default white, 100%)
  lianctl effect <ch> <name> [opts]     run a hardware effect (see 'effects')
  lianctl effects                       list effect names
  lianctl sync <on|off>                 toggle motherboard ARGB-header sync
  lianctl daemon [--config PATH]        run fan curves from the config file
  lianctl config init [--config PATH]   write a default config file

effect opts: bri=<0-100> speed=<0-100> dir=<ltr|rtl> color=<#RRGGBB>
`

func main() {
	if len(os.Args) < 2 {
		fmt.Print(usage)
		os.Exit(2)
	}
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	switch args[0] {
	case "-h", "--help", "help":
		fmt.Print(usage)
		return nil
	case "list":
		return cmdList()
	case "sensors":
		for _, s := range sensors.ListHwmon() {
			fmt.Println(s)
		}
		return nil
	case "temp":
		return cmdTemp()
	case "effects":
		fmt.Println(strings.Join(device.EffectModeNames(), "\n"))
		return nil
	case "fan":
		return cmdFan(args[1:])
	case "speed":
		return cmdSpeed(args[1:])
	case "color":
		return cmdColor(args[1:])
	case "rgb":
		return cmdRGB(args[1:])
	case "effect":
		return cmdEffect(args[1:])
	case "sync":
		return cmdSync(args[1:])
	case "config":
		return cmdConfig(args[1:])
	case "daemon":
		return cmdDaemon(args[1:])
	default:
		return fmt.Errorf("unknown command %q (try 'lianctl help')", args[0])
	}
}

func openAll() ([]device.Controller, error) {
	ctrls, err := device.OpenAll()
	if err != nil {
		return nil, err
	}
	if len(ctrls) == 0 {
		return nil, fmt.Errorf("no Lian Li controllers found (is one connected? is the udev rule installed?)")
	}
	return ctrls, nil
}

func cmdList() error {
	ctrls, err := openAll()
	if err != nil {
		return err
	}
	defer closeAll(ctrls)
	for _, c := range ctrls {
		info := c.Info()
		fmt.Printf("%s  [%04x:%04x]  %s  (%d fan ch, %d rgb ch)\n",
			c.Model().Name, info.VendorID, info.ProductID, info.Path,
			c.FanChannels(), c.RGBChannels())
	}
	return nil
}

func cmdFan(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("usage: lianctl fan <ch> <percent|pwm>")
	}
	ch, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("bad channel %q", args[0])
	}
	ctrls, err := openAll()
	if err != nil {
		return err
	}
	defer closeAll(ctrls)

	if strings.EqualFold(args[1], "pwm") {
		return forEach(ctrls, ch, true, func(c device.Controller) error {
			return c.SetFanPWM(ch)
		})
	}
	pct, err := strconv.ParseFloat(args[1], 64)
	if err != nil {
		return fmt.Errorf("bad percent %q", args[1])
	}
	return forEach(ctrls, ch, true, func(c device.Controller) error {
		return c.SetFanPercent(ch, pct)
	})
}

// tempStatus classifies a temperature into a status word and an ANSI color.
func tempStatus(c float64) (status, color string) {
	switch {
	case c >= 85:
		return "TOO HOT", "\033[1;31m" // bold red
	case c >= 70:
		return "HOT", "\033[31m" // red
	case c >= 55:
		return "WARM", "\033[33m" // yellow
	default:
		return "OK", "\033[32m" // green
	}
}

// cmdTemp prints every hwmon temperature with a color-coded heat status.
func cmdTemp() error {
	readings := sensors.ReadAll()
	if len(readings) == 0 {
		return fmt.Errorf("no hwmon temperatures found")
	}
	color := isTerminal(os.Stdout)
	hottest := -300.0
	for _, r := range readings {
		printTemp(r.Name(), r.Celsius, color)
		if r.Celsius > hottest {
			hottest = r.Celsius
		}
	}
	fmt.Println()
	printTemp("hottest", hottest, color)
	return nil
}

func printTemp(name string, c float64, color bool) {
	status, col := tempStatus(c)
	if color {
		fmt.Printf("%-28s %6.1f°C  %s%s\033[0m\n", name, c, col, status)
	} else {
		fmt.Printf("%-28s %6.1f°C  %s\n", name, c, status)
	}
}

// isTerminal reports whether f is attached to a terminal (so we only emit ANSI
// color when a human is watching, not when piped to a file).
func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// speedPresets maps friendly names to fan duty percentages.
var speedPresets = map[string]float64{
	"low":    30,
	"medium": 50,
	"high":   75,
	"full":   100,
}

// cmdSpeed sets every fan channel on every controller to a named preset duty.
func cmdSpeed(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: lianctl speed <low|medium|high|full>")
	}
	pct, ok := speedPresets[strings.ToLower(args[0])]
	if !ok {
		return fmt.Errorf("unknown speed %q (try: low, medium, high, full)", args[0])
	}
	ctrls, err := openAll()
	if err != nil {
		return err
	}
	defer closeAll(ctrls)
	for _, c := range ctrls {
		for ch := 0; ch < c.FanChannels(); ch++ {
			if err := c.SetFanPercent(ch, pct); err != nil {
				return err
			}
		}
	}
	return nil
}

func cmdColor(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: lianctl color <ch> <#RRGGBB> [brightness]")
	}
	ch, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("bad channel %q", args[0])
	}
	col, err := device.ParseHexColor(args[1])
	if err != nil {
		return err
	}
	bri := 100.0
	if len(args) >= 3 {
		if bri, err = strconv.ParseFloat(args[2], 64); err != nil {
			return fmt.Errorf("bad brightness %q", args[2])
		}
	}
	colors := make([]device.Color, 96)
	for i := range colors {
		colors[i] = col
	}
	ctrls, err := openAll()
	if err != nil {
		return err
	}
	defer closeAll(ctrls)
	return forEach(ctrls, ch, false, func(c device.Controller) error {
		return c.SetChannelColors(ch, colors, bri)
	})
}

// cmdRGB is the simple all-channels power switch: `rgb off` blanks everything,
// `rgb on [#RRGGBB] [bri]` lights every channel a solid color (default white).
func cmdRGB(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: lianctl rgb <off|on> [#RRGGBB] [brightness]")
	}
	switch strings.ToLower(args[0]) {
	case "off":
		if len(args) != 1 {
			return fmt.Errorf("usage: lianctl rgb off")
		}
		return setAllRGB(false, device.Color{}, 0)
	case "on":
		col := device.Color{R: 0xff, G: 0xff, B: 0xff}
		if len(args) >= 2 {
			c, err := device.ParseHexColor(args[1])
			if err != nil {
				return err
			}
			col = c
		}
		bri := 100.0
		if len(args) >= 3 {
			var err error
			if bri, err = strconv.ParseFloat(args[2], 64); err != nil {
				return fmt.Errorf("bad brightness %q", args[2])
			}
		}
		return setAllRGB(true, col, bri)
	default:
		return fmt.Errorf("usage: lianctl rgb <off|on> [#RRGGBB] [brightness]")
	}
}

// setAllRGB applies a single color (or off) to every RGB channel on every
// controller. Per-LED writes only take effect with motherboard sync disabled,
// so we always clear sync first — that alone is what turns off models that
// don't support per-LED control.
func setAllRGB(on bool, col device.Color, bri float64) error {
	ctrls, err := openAll()
	if err != nil {
		return err
	}
	defer closeAll(ctrls)

	colors := fillColor(col, 96)
	for _, c := range ctrls {
		if err := c.SetMotherboardSync(false); err != nil {
			return err
		}
		for ch := 0; ch < c.RGBChannels(); ch++ {
			err := c.SetChannelColors(ch, colors, bri)
			if errors.Is(err, device.ErrUnsupported) {
				// Sync-only model: no per-LED control. `off` is already done
				// (sync cleared above); for `on`, re-enable sync so the ARGB
				// header drives the LEDs again.
				if on {
					if err := c.SetMotherboardSync(true); err != nil {
						return err
					}
				}
				break
			}
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func cmdEffect(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: lianctl effect <ch> <name> [bri= speed= dir= color=]")
	}
	ch, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("bad channel %q", args[0])
	}
	eff := device.Effect{Mode: args[1], Brightness: 100, Speed: 50}
	for _, opt := range args[2:] {
		k, v, ok := strings.Cut(opt, "=")
		if !ok {
			return fmt.Errorf("bad option %q (want key=value)", opt)
		}
		switch k {
		case "bri":
			eff.Brightness, err = strconv.ParseFloat(v, 64)
		case "speed":
			eff.Speed, err = strconv.ParseFloat(v, 64)
		case "dir":
			if strings.EqualFold(v, "rtl") {
				eff.Direction = device.DirRTL
			}
		case "color":
			var col device.Color
			col, err = device.ParseHexColor(v)
			if err == nil {
				eff.Colors = fillColor(col, 96)
			}
		default:
			return fmt.Errorf("unknown option %q", k)
		}
		if err != nil {
			return err
		}
	}
	ctrls, err := openAll()
	if err != nil {
		return err
	}
	defer closeAll(ctrls)
	return forEach(ctrls, ch, false, func(c device.Controller) error {
		return c.SetChannelEffect(ch, eff)
	})
}

func cmdSync(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: lianctl sync <on|off>")
	}
	on := strings.EqualFold(args[0], "on")
	ctrls, err := openAll()
	if err != nil {
		return err
	}
	defer closeAll(ctrls)
	for _, c := range ctrls {
		if err := c.SetMotherboardSync(on); err != nil {
			return err
		}
	}
	return nil
}

func cmdConfig(args []string) error {
	if len(args) == 0 || args[0] != "init" {
		return fmt.Errorf("usage: lianctl config init [--config PATH]")
	}
	path := configPath(args[1:])
	if err := config.Save(path, config.Default()); err != nil {
		return err
	}
	fmt.Printf("wrote default config to %s\n", path)
	return nil
}

func cmdDaemon(args []string) error {
	path := configPath(args)
	cfg, err := config.Load(path)
	if err != nil {
		return fmt.Errorf("loading config %s: %w (run 'lianctl config init' first)", path, err)
	}
	ctrls, err := openAll()
	if err != nil {
		return err
	}
	defer closeAll(ctrls)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := daemon.Run(ctx, ctrls, cfg); err != nil && err != context.Canceled {
		return err
	}
	return nil
}

// ---- helpers ----

func configPath(args []string) string {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "--config" {
			return args[i+1]
		}
	}
	return config.DefaultPath()
}

// forEach applies fn to every controller that owns the given channel. fan
// selects which channel-count to range-check against.
func forEach(ctrls []device.Controller, ch int, fan bool, fn func(device.Controller) error) error {
	applied := 0
	for _, c := range ctrls {
		limit := c.RGBChannels()
		if fan {
			limit = c.FanChannels()
		}
		if ch < 0 || ch >= limit {
			continue
		}
		if err := fn(c); err != nil {
			return err
		}
		applied++
	}
	if applied == 0 {
		return fmt.Errorf("no controller has channel %d", ch)
	}
	return nil
}

func fillColor(c device.Color, n int) []device.Color {
	out := make([]device.Color, n)
	for i := range out {
		out[i] = c
	}
	return out
}

func closeAll(ctrls []device.Controller) {
	for _, c := range ctrls {
		_ = c.Close()
	}
}
