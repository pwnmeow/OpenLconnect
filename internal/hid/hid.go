// Package hid is a small, dependency-free HID transport for talking to
// Lian Li USB controllers.
//
// The real implementation lives in hidraw_linux.go (pure Go over /dev/hidraw*,
// no cgo, no libusb). Other platforms get a stub so the project still builds
// and `go vet`s while you develop on them — but lianctl is a Linux tool.
package hid

import "errors"

// ErrUnsupportedOS is returned by the transport on non-Linux platforms.
var ErrUnsupportedOS = errors.New("lianctl HID transport is only implemented on Linux (hidraw)")

// Info describes an enumerated HID device.
type Info struct {
	Path      string // e.g. /dev/hidraw3
	VendorID  uint16
	ProductID uint16
	Name      string // HID_NAME from the kernel, best-effort
}

// Device is an open handle to a HID device.
type Device interface {
	// Write sends a single output report. The first byte of buf is the
	// report ID (Lian Li controllers use 0xE0).
	Write(buf []byte) (int, error)
	// Read reads an input report with the given timeout. May be unused for
	// write-only control flows.
	Read(buf []byte, timeoutMS int) (int, error)
	Close() error
}

// Enumerate returns every present HID device whose VID/PID matches one of the
// given (vid, pid) pairs. Pass a nil pids slice to match any PID for a vendor.
func Enumerate(vendorID uint16, productIDs []uint16) ([]Info, error) {
	return enumerate(vendorID, productIDs)
}

// Open opens the device at the given hidraw path.
func Open(path string) (Device, error) {
	return open(path)
}

func contains(s []uint16, v uint16) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
