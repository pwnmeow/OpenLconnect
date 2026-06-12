//go:build linux

package hid

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// enumerate walks /sys/class/hidraw and matches devices by VID/PID parsed from
// each device's uevent (HID_ID=bus:vid:pid).
func enumerate(vendorID uint16, productIDs []uint16) ([]Info, error) {
	const base = "/sys/class/hidraw"
	entries, err := os.ReadDir(base)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", base, err)
	}

	var out []Info
	for _, e := range entries {
		name := e.Name() // hidrawN
		uevent := filepath.Join(base, name, "device", "uevent")
		data, err := os.ReadFile(uevent)
		if err != nil {
			continue
		}
		vid, pid, hidName, ok := parseUevent(string(data))
		if !ok || vid != vendorID {
			continue
		}
		if len(productIDs) > 0 && !contains(productIDs, pid) {
			continue
		}
		out = append(out, Info{
			Path:      filepath.Join("/dev", name),
			VendorID:  vid,
			ProductID: pid,
			Name:      hidName,
		})
	}
	return out, nil
}

// parseUevent extracts vid/pid/name from a hidraw uevent file.
// The relevant line looks like: HID_ID=0003:00000CF2:0000A102
func parseUevent(s string) (vid, pid uint16, name string, ok bool) {
	for _, line := range strings.Split(s, "\n") {
		switch {
		case strings.HasPrefix(line, "HID_ID="):
			parts := strings.Split(strings.TrimPrefix(line, "HID_ID="), ":")
			if len(parts) != 3 {
				continue
			}
			v, err1 := strconv.ParseUint(strings.TrimSpace(parts[1]), 16, 32)
			p, err2 := strconv.ParseUint(strings.TrimSpace(parts[2]), 16, 32)
			if err1 == nil && err2 == nil {
				vid, pid = uint16(v), uint16(p)
				ok = true
			}
		case strings.HasPrefix(line, "HID_NAME="):
			name = strings.TrimSpace(strings.TrimPrefix(line, "HID_NAME="))
		}
	}
	return vid, pid, name, ok
}

type linuxDevice struct {
	f *os.File
}

func open(path string) (Device, error) {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w (is the udev rule installed / are you in the right group?)", path, err)
	}
	return &linuxDevice{f: f}, nil
}

func (d *linuxDevice) Write(buf []byte) (int, error) {
	// A plain write() to a hidraw node sends an output report; the first byte
	// is the report ID. Lian Li controllers use report ID 0xE0.
	return d.f.Write(buf)
}

func (d *linuxDevice) Read(buf []byte, timeoutMS int) (int, error) {
	if timeoutMS > 0 {
		fd := int(d.f.Fd())
		var fdset syscall.FdSet
		fdSet(&fdset, fd)
		tv := syscall.NsecToTimeval(int64(timeoutMS) * int64(time.Millisecond))
		n, err := syscall.Select(fd+1, &fdset, nil, nil, &tv)
		if err != nil {
			return 0, err
		}
		if n == 0 {
			return 0, os.ErrDeadlineExceeded
		}
	}
	return d.f.Read(buf)
}

func (d *linuxDevice) Close() error { return d.f.Close() }

func fdSet(p *syscall.FdSet, fd int) {
	p.Bits[fd/64] |= 1 << (uint(fd) % 64)
}
