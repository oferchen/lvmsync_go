package dump

import (
	"testing"

	cpufeatures "lvmsync_go/internal/cpufeatures"
	"lvmsync_go/transfer"
)

func TestChooseCompressionExplicit(t *testing.T) {
	if got := chooseCompression(0, "lz4"); got != "lz4" {
		t.Fatalf("chooseCompression explicit lz4 = %q, want lz4", got)
	}
	if got := chooseCompression(0, "zstd"); got != "zstd" {
		t.Fatalf("chooseCompression explicit zstd = %q, want zstd", got)
	}
}

func TestChooseCompressionAutoWithZstdSupport(t *testing.T) {
	if !(cpufeatures.HasAVX2() || cpufeatures.HasNEON()) {
		t.Skip("zstd not supported")
	}
	if got := chooseCompression(0, transfer.StrategyAuto); got != "zstd" {
		t.Fatalf("chooseCompression auto with zstd support = %q, want zstd", got)
	}
}

func TestChooseCompressionAutoWithoutZstdSupport(t *testing.T) {
	if cpufeatures.HasAVX2() || cpufeatures.HasNEON() {
		t.Skip("zstd supported")
	}
	if got := chooseCompression(0, transfer.StrategyAuto); got != "lz4" {
		t.Fatalf("chooseCompression auto without zstd support = %q, want lz4", got)
	}
}

func TestChooseCompressionSmallChunk(t *testing.T) {
	want := "lz4"
	if cpufeatures.HasAVX2() || cpufeatures.HasNEON() {
		want = "zstd"
	}
	if got := chooseCompression(32*1024, transfer.StrategyAuto); got != want {
		t.Fatalf("chooseCompression small chunk = %q, want %s", got, want)
	}
}
