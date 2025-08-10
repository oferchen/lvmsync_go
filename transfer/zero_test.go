package transfer

import "testing"

func TestIsAllZero(t *testing.T) {
	if !isAllZero([]byte{0, 0, 0}) {
		t.Fatalf("expected zero slice to be detected")
	}
	if isAllZero([]byte{0, 1, 0}) {
		t.Fatalf("expected non-zero slice to be detected")
	}
}
