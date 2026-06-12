package config

import "testing"

func TestInterp(t *testing.T) {
	r := FanRule{Curve: [][2]float64{{30, 30}, {60, 60}, {75, 100}}}
	cases := []struct {
		temp, want float64
	}{
		{20, 30},   // below first point -> clamp low
		{30, 30},   // exact
		{45, 45},   // midpoint 30..60
		{60, 60},   // exact
		{67.5, 80}, // midpoint 60..75
		{90, 100},  // above last -> clamp high
	}
	for _, c := range cases {
		if got := r.Interp(c.temp); got != c.want {
			t.Errorf("Interp(%.1f) = %.2f, want %.2f", c.temp, got, c.want)
		}
	}
}

func TestInterpEmpty(t *testing.T) {
	if got := (FanRule{}).Interp(50); got != 0 {
		t.Errorf("empty curve = %v, want 0", got)
	}
}
