package transfer

import (
	"bytes"
	"context"
	"sync"
	"testing"

	"github.com/oferchen/lvmsync_go/internal/config"

	"go.uber.org/zap"
)

func BenchmarkDumpChangesSequential(b *testing.B) {
	tr := NewTransfer(zap.NewNop(), &sync.WaitGroup{}, nil)
	blockSize := int64(4096)
	changed := []int{0, 1, 2, 3}
	src, snapshot := createDumpTestFiles(b, blockSize, changed)
	cfg := &config.Config{BlockSize: int(blockSize), Compress: "none", MaxRetries: 1}
	var buf bytes.Buffer
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		if err := tr.DumpChangesSequential(context.Background(), cfg, snapshot, src, &buf); err != nil {
			b.Fatalf("DumpChangesSequential failed: %v", err)
		}
	}
}

func BenchmarkDumpChangesParallel(b *testing.B) {
	tr := NewTransfer(zap.NewNop(), &sync.WaitGroup{}, nil)
	blockSize := int64(4096)
	changed := []int{0, 1, 2, 3}
	src, snapshot := createDumpTestFiles(b, blockSize, changed)
	cfg := &config.Config{BlockSize: int(blockSize), Compress: "none", Parallel: 1, VerifyChecksum: true, MaxRetries: 1}
	var buf bytes.Buffer
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		if err := tr.DumpChangesParallel(context.Background(), cfg, snapshot, src, &buf); err != nil {
			b.Fatalf("DumpChangesParallel failed: %v", err)
		}
	}
}
