//go:build !linux

package hid

// Stub backend for non-Linux platforms so the project builds while developing
// on Windows/macOS. lianctl only does real I/O on Linux (hidraw).

func enumerate(vendorID uint16, productIDs []uint16) ([]Info, error) {
	return nil, ErrUnsupportedOS
}

func open(path string) (Device, error) {
	return nil, ErrUnsupportedOS
}
