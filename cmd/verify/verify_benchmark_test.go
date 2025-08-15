package verify

import (
	"io"
	"os"
	"testing"

	"go.uber.org/zap"

	"lvmsync_go/config"
	"lvmsync_go/transfer"
)

func createBenchFiles(b *testing.B, blockSize, blocks int) (string, string) {
	b.Helper()
	size := blockSize * blocks
	src, dst := createTestFile(b, size), createTestFile(b, size)
	return src, dst
}

// verifyFullAlloc mimics the pre-optimized verification by allocating buffers each iteration.
func verifyFullAlloc(cfg *config.Config, src, dst string) error {
	blockSize := cfg.BlockSize
	fSrc, err := os.Open(src)
	if err != nil {
		return err
	}
	defer fSrc.Close()
	fDst, err := os.Open(dst)
	if err != nil {
		return err
	}
	defer fDst.Close()
	info, err := fSrc.Stat()
	if err != nil {
		return err
	}
	total := info.Size()
	for off := int64(0); off < total; off += int64(blockSize) {
		if _, err := transfer.ReadBlock(fSrc, off, blockSize); err != nil && err != io.EOF {
			return err
		}
		if _, err := transfer.ReadBlock(fDst, off, blockSize); err != nil && err != io.EOF {
			return err
		}
	}
	return nil
}

func BenchmarkVerifyFullAlloc(b *testing.B) {
	blockSize, blocks := 4096, 16
	src, dst := createBenchFiles(b, blockSize, blocks)
	cfg := &config.Config{BlockSize: blockSize}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if err := verifyFullAlloc(cfg, src, dst); err != nil {
			b.Fatalf("verifyFullAlloc: %v", err)
		}
	}
}

func BenchmarkVerifyFull(b *testing.B) {
	blockSize, blocks := 4096, 16
	src, dst := createBenchFiles(b, blockSize, blocks)
	cfg := &config.Config{BlockSize: blockSize}
	b.ReportAllocs()
	logger := zap.NewNop()
	for i := 0; i < b.N; i++ {
		if err := verifyFull(cfg, src, dst, logger); err != nil {
			b.Fatalf("verifyFull: %v", err)
		}
	}
}
