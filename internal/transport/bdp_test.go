package transport

import (
	"testing"
	"time"
)

func TestBDPTrackerAdjustsConcurrency(t *testing.T) {
	tracker := NewBDPTracker(64*1024, 0)
	cases := []struct {
		rtt  time.Duration
		bw   float64
		want int
	}{
		{10 * time.Millisecond, 100 * 1024 * 1024, 24},
		{100 * time.Millisecond, 10 * 1024 * 1024, 24},
		{50 * time.Millisecond, 1 * 1024 * 1024, 1},
	}
	for i, tt := range cases {
		got := tracker.Update(tt.rtt, tt.bw)
		if got != tt.want {
			t.Fatalf("case %d got %d want %d", i, got, tt.want)
		}
	}
}

func TestBDPTrackerOverride(t *testing.T) {
	tracker := NewBDPTracker(64*1024, 5)
	if got := tracker.Update(10*time.Millisecond, 100*1024*1024); got != 5 {
		t.Fatalf("override got %d want 5", got)
	}
	tracker.Override(0)
	if got := tracker.Update(10*time.Millisecond, 100*1024*1024); got != 24 {
		t.Fatalf("auto got %d want 24", got)
	}
}
