package transfer

import "testing"

func TestEstimate(t *testing.T) {
	dur, bw := Estimate(1000, 100)
	if dur != 10000 {
		t.Fatalf("duration %d", dur)
	}
	if bw != 800 {
		t.Fatalf("bandwidth %d", bw)
	}
}

func TestEstimateZero(t *testing.T) {
	dur, bw := Estimate(0, 0)
	if dur != 0 || bw != 0 {
		t.Fatalf("expected zeros got %d %d", dur, bw)
	}
}
