package transfer

import (
	"bytes"
	"crypto/rand"
	"testing"

	"github.com/pierrec/lz4/v4"
	"go.uber.org/zap"
)

func fullRatio(data []byte) (float64, error) {
	var buf bytes.Buffer
	w, err := NewCompressionWriter(&buf, compressionLZ4, int(lz4.Level1), 1)
	if err != nil {
		return 0, err
	}
	if _, err := w.Write(data); err != nil {
		_ = w.Close()
		return 0, err
	}
	if err := w.Close(); err != nil {
		return 0, err
	}
	return float64(buf.Len()) / float64(len(data)), nil
}

func TestCompressionSamplingDecisions(t *testing.T) {
	random := make([]byte, 64*1024)
	if _, err := rand.Read(random); err != nil {
		t.Fatalf("rand read failed: %v", err)
	}
	var preBuf bytes.Buffer
	preW := lz4.NewWriter(&preBuf)
	if _, err := preW.Write(random); err != nil {
		t.Fatalf("precompress write failed: %v", err)
	}
	if err := preW.Close(); err != nil {
		t.Fatalf("precompress close failed: %v", err)
	}
	precompressed := preBuf.Bytes()

	cases := []struct {
		name         string
		data         []byte
		wantCompress bool
	}{
		{"zeros", bytes.Repeat([]byte{0}, 64*1024), true},
		{"random", random, false},
		{"precompressed", precompressed, false},
	}

	const threshold = 0.9
	for _, tc := range cases {
		full, err := fullRatio(tc.data)
		if err != nil {
			t.Fatalf("%s full ratio error: %v", tc.name, err)
		}
		sample, err := estimateRatio(tc.data, compressionLZ4, int(lz4.Level1), 1)
		if err != nil {
			t.Fatalf("%s sample ratio error: %v", tc.name, err)
		}
		fullDec := full < threshold
		sampleDec := sample < threshold
		if fullDec != sampleDec {
			t.Fatalf("%s decision mismatch: full %v sample %v", tc.name, fullDec, sampleDec)
		}
		if fullDec != tc.wantCompress {
			t.Fatalf("%s expected compress=%v, got %v", tc.name, tc.wantCompress, fullDec)
		}
	}
}

func TestSmallHighlyCompressibleBlock(t *testing.T) {
	data := bytes.Repeat([]byte{0}, 4*1024)
	out, algo, err := CompressChunk(data, StrategyAuto, 0, 0, 1, 0.9, zap.NewNop())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if algo == "none" {
		t.Fatalf("expected compression for small block")
	}
	if len(out) >= len(data) {
		t.Fatalf("compressed output should be smaller than input")
	}
}
