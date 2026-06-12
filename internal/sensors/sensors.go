// Package sensors reads temperatures to drive fan curves.
package sensors

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// Read resolves a temperature source string to degrees Celsius.
//
// Supported forms:
//
//	hwmon:<chip>/<label>   e.g. hwmon:k10temp/Tctl  or  hwmon:coretemp/Package id 0
//	file:/sys/.../temp1_input   raw sysfs millidegree file
//	cmd:<shell command>    stdout parsed as a float (degrees C)
//	max                    the hottest hwmon reading on the system
func Read(source string) (float64, error) {
	switch {
	case source == "max":
		return readMax()
	case strings.HasPrefix(source, "file:"):
		return readSysfsTemp(strings.TrimPrefix(source, "file:"))
	case strings.HasPrefix(source, "cmd:"):
		return readCmd(strings.TrimPrefix(source, "cmd:"))
	case strings.HasPrefix(source, "hwmon:"):
		return readHwmonLabel(strings.TrimPrefix(source, "hwmon:"))
	default:
		return 0, fmt.Errorf("unrecognized temperature source %q", source)
	}
}

func readSysfsTemp(path string) (float64, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	milli, err := strconv.ParseFloat(strings.TrimSpace(string(b)), 64)
	if err != nil {
		return 0, fmt.Errorf("parsing %s: %w", path, err)
	}
	return milli / 1000.0, nil
}

func readCmd(command string) (float64, error) {
	out, err := exec.Command("/bin/sh", "-c", command).Output()
	if err != nil {
		return 0, fmt.Errorf("temp command failed: %w", err)
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil {
		return 0, fmt.Errorf("temp command output %q not a number: %w", strings.TrimSpace(string(out)), err)
	}
	return v, nil
}

// readHwmonLabel finds /sys/class/hwmon/hwmonN where name==chip and returns the
// tempX_input whose tempX_label matches label (case-insensitive substring).
func readHwmonLabel(spec string) (float64, error) {
	chip, label, found := strings.Cut(spec, "/")
	if !found {
		return 0, fmt.Errorf("hwmon source must be hwmon:<chip>/<label>, got %q", spec)
	}
	dirs, _ := filepath.Glob("/sys/class/hwmon/hwmon*")
	for _, d := range dirs {
		name, err := os.ReadFile(filepath.Join(d, "name"))
		if err != nil || strings.TrimSpace(string(name)) != chip {
			continue
		}
		labels, _ := filepath.Glob(filepath.Join(d, "temp*_label"))
		for _, lf := range labels {
			lb, _ := os.ReadFile(lf)
			if strings.Contains(strings.ToLower(strings.TrimSpace(string(lb))), strings.ToLower(label)) {
				input := strings.TrimSuffix(lf, "_label") + "_input"
				return readSysfsTemp(input)
			}
		}
		// No labels: fall back to temp1_input.
		if label == "" {
			return readSysfsTemp(filepath.Join(d, "temp1_input"))
		}
	}
	return 0, fmt.Errorf("hwmon chip %q label %q not found", chip, label)
}

func readMax() (float64, error) {
	inputs, _ := filepath.Glob("/sys/class/hwmon/hwmon*/temp*_input")
	max := -300.0
	for _, in := range inputs {
		v, err := readSysfsTemp(in)
		if err == nil && v > max {
			max = v
		}
	}
	if max < -200 {
		return 0, fmt.Errorf("no hwmon temperatures found")
	}
	return max, nil
}

// ListHwmon returns a human-readable inventory of available hwmon temperatures,
// for `lianctl sensors`.
func ListHwmon() []string {
	var out []string
	dirs, _ := filepath.Glob("/sys/class/hwmon/hwmon*")
	for _, d := range dirs {
		name, _ := os.ReadFile(filepath.Join(d, "name"))
		chip := strings.TrimSpace(string(name))
		inputs, _ := filepath.Glob(filepath.Join(d, "temp*_input"))
		for _, in := range inputs {
			label := strings.TrimSuffix(in, "_input") + "_label"
			lb, _ := os.ReadFile(label)
			l := strings.TrimSpace(string(lb))
			v, err := readSysfsTemp(in)
			if err != nil {
				continue
			}
			if l == "" {
				out = append(out, fmt.Sprintf("hwmon:%s/  -> %.1f°C  (%s)", chip, v, in))
			} else {
				out = append(out, fmt.Sprintf("hwmon:%s/%s -> %.1f°C", chip, l, v))
			}
		}
	}
	return out
}
