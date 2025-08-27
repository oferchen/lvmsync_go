//go:build integration

package integration

import (
	"bytes"
	"compress/gzip"
	crand "crypto/rand"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/oferchen/lvmsync_go/transfer"
)

// generateGzip returns a gzip-compressed blob of random data of the given size.
func generateGzip(t *testing.T, size int) []byte {
	t.Helper()
	src := make([]byte, size)
	if _, err := crand.Read(src); err != nil {
		t.Fatalf("rand read failed: %v", err)
	}
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(src); err != nil {
		t.Fatalf("gzip write failed: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("gzip close failed: %v", err)
	}
	return buf.Bytes()
}

func throughput(data []byte, t *testing.T) (string, float64) {
	start := time.Now()
	_, used, err := transfer.CompressChunk(data, transfer.StrategyAuto, 0, 0, 1, 0.9, zap.NewNop())
	if err != nil {
		t.Fatalf("compress chunk: %v", err)
	}
	dur := time.Since(start).Seconds()
	return used, float64(len(data)) / dur / 1048576.0
}

func TestCompressionSkip(t *testing.T) {
	requireRootAndCommands(t)
	pre := generateGzip(t, 64*1024)
	zeros := bytes.Repeat([]byte{0}, 64*1024)

	algoPre, thrPre := throughput(pre, t)
	if algoPre != "none" {
		t.Fatalf("expected none for precompressed data, got %s", algoPre)
	}
	algoZero, thrZero := throughput(zeros, t)
	if algoZero == "none" {
		t.Fatalf("expected compression for zero data")
	}
	t.Logf("precompressed_throughput=%.2fMB/s zeros_throughput=%.2fMB/s", thrPre, thrZero)
}
