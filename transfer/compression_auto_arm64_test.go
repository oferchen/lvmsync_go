//go:build arm64

package transfer

import (
	"io"
	"testing"

	zstd "github.com/klauspost/compress/zstd"
	"github.com/klauspost/cpuid/v2"
	"github.com/pierrec/lz4/v4"
)

func TestNewCompressionWriterAutoARM64(t *testing.T) {
	original := cpuid.CPU
	defer func() { cpuid.CPU = original }()

	t.Run("neon", func(t *testing.T) {
		cpuid.CPU = cpuid.CPUInfo{}
		cpuid.CPU.Enable(cpuid.ASIMD)
		w, err := NewCompressionWriter(io.Discard, "auto", 1)
		if err != nil {
			t.Fatalf("NewCompressionWriter with ASIMD: %v", err)
		}
		if _, ok := w.(*zstd.Encoder); !ok {
			t.Fatalf("expected zstd writer when ASIMD is present")
		}
		w.Close()
	})

	t.Run("fallback", func(t *testing.T) {
		cpuid.CPU = cpuid.CPUInfo{}
		w, err := NewCompressionWriter(io.Discard, "auto", int(lz4.Level1))
		if err != nil {
			t.Fatalf("NewCompressionWriter without ASIMD: %v", err)
		}
		if _, ok := w.(*lz4.Writer); !ok {
			t.Fatalf("expected lz4 writer when ASIMD is absent")
		}
		w.Close()
	})
}
