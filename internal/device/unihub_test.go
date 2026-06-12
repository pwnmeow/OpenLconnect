package device

import (
	"testing"

	"github.com/pwnmeow/OpenLconnect/internal/hid"
)

// fakeDevice records every write so we can assert on the wire bytes.
type fakeDevice struct{ writes [][]byte }

func (f *fakeDevice) Write(b []byte) (int, error) {
	cp := append([]byte(nil), b...)
	f.writes = append(f.writes, cp)
	return len(b), nil
}
func (f *fakeDevice) Read([]byte, int) (int, error) { return 0, nil }
func (f *fakeDevice) Close() error                  { return nil }

func newTestHub(pid uint16) (*uniHub, *fakeDevice) {
	fd := &fakeDevice{}
	m, _ := modelFor(pid)
	return newUniHub(hid.Info{ProductID: pid}, m, fd), fd
}

func TestSLInfinityFanSpeed(t *testing.T) {
	h, fd := newTestHub(0xA102)
	if err := h.SetFanPercent(1, 100); err != nil {
		t.Fatal(err)
	}
	// Expect: mode write then speed write.
	if len(fd.writes) != 2 {
		t.Fatalf("got %d writes, want 2", len(fd.writes))
	}
	// Mode write: [E0,10,62, 0x10<<1=0x20]
	wantMode := []byte{0xE0, 0x10, 0x62, 0x20}
	assertBytes(t, "mode", fd.writes[0], wantMode)

	// Speed write: [E0, 0x20+1, 0, (200+19*100)/21 = 2100/21 = 100]
	wantSpeed := []byte{0xE0, 0x21, 0x00, 100}
	assertBytes(t, "speed", fd.writes[1], wantSpeed)
}

func TestSLInfinityFanSpeedZero(t *testing.T) {
	h, fd := newTestHub(0xA102)
	_ = h.SetFanPercent(0, 0)
	// (200 + 0)/21 = 9
	if got := fd.writes[1][3]; got != 9 {
		t.Errorf("0%% speed byte = %d, want 9", got)
	}
}

func TestSLInfinityPWM(t *testing.T) {
	h, fd := newTestHub(0xA102)
	_ = h.SetFanPWM(2)
	// channel_byte = (0x10<<2) | (1<<2) = 0x40 | 0x04 = 0x44
	assertBytes(t, "pwm", fd.writes[0], []byte{0xE0, 0x10, 0x62, 0x44})
}

func TestSLInfinityColors(t *testing.T) {
	h, fd := newTestHub(0xA102)
	colors := []Color{{R: 0x11, G: 0x22, B: 0x33}}
	if err := h.SetChannelColors(3, colors, 100); err != nil {
		t.Fatal(err)
	}
	// start, colors, commit
	if len(fd.writes) != 3 {
		t.Fatalf("got %d writes, want 3", len(fd.writes))
	}
	// Start: [E0,10,60, 1+3/2=2, 04]
	assertPrefix(t, "start", fd.writes[0], []byte{0xE0, 0x10, 0x60, 0x02, 0x04})
	if len(fd.writes[0]) != slinfActionLen {
		t.Errorf("start len = %d, want %d", len(fd.writes[0]), slinfActionLen)
	}
	// Colors: [E0, 0x30+3=0x33, R,B,G ...]
	col := fd.writes[1]
	if len(col) != slinfColorPacketLen {
		t.Errorf("color len = %d, want %d", len(col), slinfColorPacketLen)
	}
	assertPrefix(t, "color", col, []byte{0xE0, 0x33, 0x11, 0x33, 0x22}) // R,B,G order
	// Commit: [E0, 0x10+3=0x13, mode static=0x01, speed, dir, bri=0x00]
	assertPrefix(t, "commit", fd.writes[2], []byte{0xE0, 0x13, 0x01})
	if fd.writes[2][5] != 0x00 { // brightness 100 -> 0x00
		t.Errorf("brightness byte = %#x, want 0x00", fd.writes[2][5])
	}
}

func TestSyncByte(t *testing.T) {
	h, fd := newTestHub(0xA102)
	_ = h.SetMotherboardSync(true)
	assertBytes(t, "sync", fd.writes[0], []byte{0xE0, 0x10, 0x61, 0x01, 0, 0, 0})
}

func TestNonSLInfRGBUnsupported(t *testing.T) {
	h, _ := newTestHub(0xA101) // AL
	if err := h.SetChannelColors(0, []Color{{}}, 100); err == nil {
		t.Error("expected ErrUnsupported for AL per-LED RGB")
	}
}

func assertBytes(t *testing.T, name string, got, want []byte) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%s: len %d != %d (%v)", name, len(got), len(want), got)
		return
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%s: byte %d = %#x, want %#x (%v)", name, i, got[i], want[i], got)
			return
		}
	}
}

func assertPrefix(t *testing.T, name string, got, want []byte) {
	t.Helper()
	if len(got) < len(want) {
		t.Errorf("%s: too short (%d < %d)", name, len(got), len(want))
		return
	}
	assertBytes(t, name, got[:len(want)], want)
}
