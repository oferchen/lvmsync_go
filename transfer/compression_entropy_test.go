package transfer

import (
	"bytes"
	"crypto/rand"
	"testing"

	"go.uber.org/zap"

	"lvmsync_go/internal/compressiondetect"
)

// TestDetectOptimalCompressionEntropy verifies that high-entropy data is not
// compressed while low-entropy data still triggers compression when the
// algorithm is chosen via DetectOptimalCompression.
func TestDetectOptimalCompressionEntropy(t *testing.T) {
	algo := compressiondetect.DetectOptimalCompression()

	// High-entropy random data should skip compression.
	high := make([]byte, 64*1024)
	if _, err := rand.Read(high); err != nil {
		t.Fatalf("rand read failed: %v", err)
	}
	out, used, err := CompressChunk(high, algo, 0, 0, 1, 0.8, zap.NewNop())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if used != "none" {
		t.Fatalf("expected none for high entropy data, got %s", used)
	}
	if !bytes.Equal(out, high) {
		t.Fatalf("data should be unchanged when compression is skipped")
	}

	// Low-entropy data should still be compressed.
	low := bytes.Repeat([]byte{0}, 64*1024)
	out, used, err = CompressChunk(low, algo, 0, 0, 1, 0.8, zap.NewNop())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if used == "none" {
		t.Fatalf("expected compression for low entropy data")
	}
	if len(out) >= len(low) {
		t.Fatalf("compressed output should be smaller than input")
	}
}
