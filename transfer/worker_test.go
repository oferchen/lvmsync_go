package transfer

import (
	"math"
	"testing"
)

func TestCalculateTotalDataSize(t *testing.T) {
	ranges := []Range{
		{Start: 0, End: 9},
		{Start: 20, End: 29},
		{Start: 30, End: 25}, // invalid range, should be ignored
	}
	got := calculateTotalDataSize(ranges)
	want := int64(20)
	if got != want {
		t.Fatalf("calculateTotalDataSize() = %d, want %d", got, want)
	}
}

func TestCalculateTotalDataSizeOverflow(t *testing.T) {
	ranges := []Range{{Start: 0, End: uint64(math.MaxInt64)}}
	got := calculateTotalDataSize(ranges)
	if got != math.MaxInt64 {
		t.Fatalf("expected MaxInt64, got %d", got)
	}
}
