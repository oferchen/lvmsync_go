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
