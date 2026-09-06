package spot

import "testing"

func TestFrequencyHzFromMHz(t *testing.T) {
	cases := []struct {
		mhz  float64
		want int64
	}{
		{14.025, 14_025_000},
		{7.030, 7_030_000},
		{0, 0},
		{28.0001, 28_000_100},
	}
	for _, c := range cases {
		if got := FrequencyHzFromMHz(c.mhz); got != c.want {
			t.Errorf("FrequencyHzFromMHz(%v) = %d, want %d", c.mhz, got, c.want)
		}
	}
}
