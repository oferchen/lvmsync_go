//go:build amd64 || 386

package transfer

import (
	"io"
	"testing"

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
			if _, ok := w.(*pooledZstdWriter); !ok {
				t.Fatalf("expected zstd writer when %s is present", feat.String())
			}
			w.Close()
		})
	}

	t.Run("cpu-metrics", func(t *testing.T) {
		cpuid.CPU = cpuid.CPUInfo{}
		cpuid.CPU.PhysicalCores = 8
		cpuid.CPU.Cache.L3 = 8 << 20
		compressiondetect.ResetForTest()
		w, err := NewCompressionWriter(io.Discard, "auto", 1, 1)
		if err != nil {
			t.Fatalf("NewCompressionWriter with cpu metrics: %v", err)
		}
		if _, ok := w.(*pooledZstdWriter); !ok {
			t.Fatalf("expected zstd writer when cores and cache are large")
		}
		w.Close()
	})

	t.Run("fallback", func(t *testing.T) {
		cpuid.CPU = cpuid.CPUInfo{}
		cpuid.CPU.PhysicalCores = 1
		cpuid.CPU.Cache.L3 = 0
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
			if _, ok := w.(*pooledLz4Writer); !ok {
				t.Fatalf("expected lz4 writer when benchmark prefers lz4")
			}
		case compressionZSTD:
			if _, ok := w.(*pooledZstdWriter); !ok {
				t.Fatalf("expected zstd writer when benchmark prefers zstd")
			}
		}
		w.Close()
	})
}
