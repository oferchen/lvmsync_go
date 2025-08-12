package transfer

import (
	"testing"

	"github.com/zeebo/blake3"
)

func TestIsAllZero(t *testing.T) {
	if !isAllZero([]byte{0, 0, 0}) {
		t.Fatalf("expected zero slice to be detected")
	}
	if isAllZero([]byte{0, 1, 0}) {
		t.Fatalf("expected non-zero slice to be detected")
	}
}

func TestZeroHashLargeBuffer(t *testing.T) {
	size := 4096
	buf := make([]byte, size)
	if !isAllZero(buf) {
		t.Fatalf("expected zero buffer to be detected")
	}
	expected := blake3.Sum256(buf)
	if got := zeroHash(size); got != expected {
		t.Fatalf("unexpected zero hash")
	}
	buf[123] = 1
	if isAllZero(buf) {
		t.Fatalf("expected detection of non-zero buffer")
	}
}
