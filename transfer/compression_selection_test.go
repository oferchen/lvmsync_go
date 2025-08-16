package transfer

import (
	"bytes"
	"crypto/rand"
	"testing"

	"github.com/pierrec/lz4/v4"
	"go.uber.org/zap"
)

func TestSelectAlgorithm(t *testing.T) {
	origAVX2, origNEON := hasAVX2, hasNEON
	defer func() { hasAVX2, hasNEON = origAVX2, origNEON }()

	// Non-auto strategy uses provided values.
	algo, lvl := selectAlgorithm(1024, "lz4", 3, 0)
	if algo != "lz4" || lvl != 3 {
		t.Fatalf("expected lz4 level 3, got %s level %d", algo, lvl)
	}

	// Small chunks use LZ4 with default level when level is zero.
	algo, lvl = selectAlgorithm(128*1024, StrategyAuto, 0, 0)
	if algo != compressionLZ4 || lvl != int(lz4.Level1) {
		t.Fatalf("expected lz4 level1, got %s level %d", algo, lvl)
	}

	// AVX2 enables Zstd with capped level.
	hasAVX2 = func() bool { return true }
	hasNEON = func() bool { return false }
	algo, lvl = selectAlgorithm(512*1024, StrategyAuto, 0, 5)
	if algo != compressionZSTD || lvl != maxAutoZstdLv {
		t.Fatalf("expected zstd level %d, got %s level %d", maxAutoZstdLv, algo, lvl)
	}

	// NEON also enables Zstd.
	hasAVX2 = func() bool { return false }
	hasNEON = func() bool { return true }
	algo, lvl = selectAlgorithm(512*1024, StrategyAuto, 0, 0)
	if algo != compressionZSTD || lvl != defaultZstdLv {
		t.Fatalf("expected zstd level %d via NEON, got %s level %d", defaultZstdLv, algo, lvl)
	}

	// Without SIMD features, fall back to LZ4.
	hasAVX2 = func() bool { return false }
	hasNEON = func() bool { return false }
	algo, lvl = selectAlgorithm(512*1024, StrategyAuto, 0, 0)
	if algo != compressionLZ4 || lvl != int(lz4.Level1) {
		t.Fatalf("expected lz4 level1 fallback, got %s level %d", algo, lvl)
	}
}

func TestCompressChunkThreshold(t *testing.T) {
	origAVX2, origNEON := hasAVX2, hasNEON
	hasAVX2 = func() bool { return false }
	hasNEON = func() bool { return false }
	defer func() { hasAVX2, hasNEON = origAVX2, origNEON }()

	// Highly compressible data should be compressed.
	src := bytes.Repeat([]byte{0}, 64*1024)
	out, algo, err := CompressChunk(src, StrategyAuto, 0, 0, 1, 0.8, zap.NewNop())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if algo == "none" {
		t.Fatalf("expected compression, got none")
	}
	if len(out) >= len(src) {
		t.Fatalf("expected compressed output smaller than input")
	}

	// Random data should skip compression when ratio exceeds threshold.
	random := make([]byte, 64*1024)
	if _, err := rand.Read(random); err != nil {
		t.Fatalf("rand read failed: %v", err)
	}
	out, algo, err = CompressChunk(random, StrategyAuto, 0, 0, 1, 0.8, zap.NewNop())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if algo != "none" {
		t.Fatalf("expected no compression, got %s", algo)
	}
	if !bytes.Equal(out, random) {
		t.Fatalf("data should be unchanged when compression is skipped")
	}
}
