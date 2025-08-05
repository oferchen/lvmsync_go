//go:build amd64 || 386

package transfer

import (
	"io"
	"testing"

	zstd "github.com/klauspost/compress/zstd"
	"github.com/klauspost/cpuid/v2"
	"github.com/pierrec/lz4/v4"
	compressiondetect "lvmsync_go/internal/compressiondetect"
)

func TestNewCompressionWriterAutoX86(t *testing.T) {
	original := cpuid.CPU
	defer func() { cpuid.CPU = original }()

	features := []cpuid.FeatureID{cpuid.AVX512F, cpuid.AVX2, cpuid.BMI2}
	for _, feat := range features {
		t.Run(feat.String(), func(t *testing.T) {
			cpuid.CPU = cpuid.CPUInfo{}
			cpuid.CPU.Enable(feat)
			compressiondetect.ResetForTest()
			w, err := NewCompressionWriter(io.Discard, "auto", 1, 1)
			if err != nil {
				t.Fatalf("NewCompressionWriter with %s: %v", feat.String(), err)
			}
			if _, ok := w.(*zstd.Encoder); !ok {
				t.Fatalf("expected zstd writer when %s is present", feat.String())
			}
			w.Close()
		})
	}

	t.Run("fallback", func(t *testing.T) {
		cpuid.CPU = cpuid.CPUInfo{}
		compressiondetect.ResetForTest()
		algo := compressiondetect.DetectOptimalCompression()
		lvl := 1
		if algo == compressionLZ4 {
			lvl = int(lz4.Level1)
		}
		w, err := NewCompressionWriter(io.Discard, "auto", lvl, 1)
		if err != nil {
			t.Fatalf("NewCompressionWriter without features: %v", err)
		}
		switch algo {
		case compressionLZ4:
			if _, ok := w.(*lz4.Writer); !ok {
				t.Fatalf("expected lz4 writer when benchmark prefers lz4")
			}
		case compressionZSTD:
			if _, ok := w.(*zstd.Encoder); !ok {
				t.Fatalf("expected zstd writer when benchmark prefers zstd")
			}
		}
		w.Close()
	})
}
