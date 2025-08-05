package transfer

import (
	"bytes"
	"testing"

	"lvmsync_go/config"

	"go.uber.org/zap"
)

func BenchmarkDumpChangesSequential(b *testing.B) {
	SetLogger(zap.NewNop())
	blockSize := int64(4096)
	changed := []int{0, 1, 2, 3}
	src, snapshot := createDumpTestFiles(b, blockSize, changed)
	cfg := &config.Config{BlockSize: int(blockSize), Compress: "none", MaxRetries: 1}
	var buf bytes.Buffer
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		if err := DumpChangesSequential(cfg, snapshot, src, &buf); err != nil {
			b.Fatalf("DumpChangesSequential failed: %v", err)
		}
	}
}

func BenchmarkDumpChangesParallel(b *testing.B) {
	SetLogger(zap.NewNop())
	blockSize := int64(4096)
	changed := []int{0, 1, 2, 3}
	src, snapshot := createDumpTestFiles(b, blockSize, changed)
	cfg := &config.Config{BlockSize: int(blockSize), Compress: "none", Parallel: 1, VerifyChecksum: true, MaxRetries: 1}
	var buf bytes.Buffer
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		if err := DumpChangesParallel(cfg, snapshot, src, &buf); err != nil {
			b.Fatalf("DumpChangesParallel failed: %v", err)
		}
	}
}
