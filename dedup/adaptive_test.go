package dedup

import "testing"

func TestAdaptiveAvgChunkClamp(t *testing.T) {
	// clamp to minimum
	avg, _ := AdaptiveAvgChunk(100, 1<<20, 0.01, 64, 512)
	if avg != 64 {
		t.Fatalf("expected avg to clamp to min, got %d", avg)
	}
	// clamp to maximum
	avg, _ = AdaptiveAvgChunk(1<<40, 1, 0.01, 64, 1024)
	if avg != 1024 {
		t.Fatalf("expected avg to clamp to max, got %d", avg)
	}
}
