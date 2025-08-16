package compressiondetect

import "testing"

func TestBenchmarkCompression(t *testing.T) {
	algo := BenchmarkCompression()
	if algo != "lz4" && algo != "zstd" {
		t.Fatalf("unexpected algorithm: %s", algo)
	}
}

func TestDetectOptimalCompression(t *testing.T) {
	ResetForTest()
	first := DetectOptimalCompression()
	if first != "lz4" && first != "zstd" {
		t.Fatalf("unexpected algorithm: %s", first)
	}
	second := DetectOptimalCompression()
	if second != first {
		t.Fatalf("expected cached result %s, got %s", first, second)
	}
}

func TestResetForTest(t *testing.T) {
	ResetForTest()
	_ = DetectOptimalCompression()
	ResetForTest()
	if DetectOptimalCompression() == "" {
		t.Fatal("expected detection after reset")
	}
}

func TestHasAVX2(t *testing.T) {
	_ = HasAVX2()
}

func TestHasNEON(t *testing.T) {
	_ = HasNEON()
}

func TestDetectOptimalCompressionAVX2(t *testing.T) {
	ResetForTest()
	origAVX2, origNEON := hasAVX2, hasNEON
	defer func() { hasAVX2, hasNEON = origAVX2, origNEON }()
	hasAVX2 = func() bool { return true }
	hasNEON = func() bool { return false }
	if got := DetectOptimalCompression(); got != "zstd" {
		t.Fatalf("expected zstd, got %s", got)
	}
}

func TestDetectOptimalCompressionNEON(t *testing.T) {
	ResetForTest()
	origAVX2, origNEON := hasAVX2, hasNEON
	defer func() { hasAVX2, hasNEON = origAVX2, origNEON }()
	hasAVX2 = func() bool { return false }
	hasNEON = func() bool { return true }
	if got := DetectOptimalCompression(); got != "zstd" {
		t.Fatalf("expected zstd, got %s", got)
	}
}

func TestDetectOptimalCompressionBenchmarkFallback(t *testing.T) {
	ResetForTest()
	origAVX2, origNEON := hasAVX2, hasNEON
	defer func() { hasAVX2, hasNEON = origAVX2, origNEON }()
	hasAVX2 = func() bool { return false }
	hasNEON = func() bool { return false }
	got := DetectOptimalCompression()
	if got != "lz4" && got != "zstd" {
		t.Fatalf("unexpected %s", got)
	}
}
