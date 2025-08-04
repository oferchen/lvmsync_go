//go:build amd64 || 386

package transfer

import (
	"io"
	"testing"

	zstd "github.com/klauspost/compress/zstd"
	"github.com/pierrec/lz4/v4"
	"golang.org/x/sys/cpu"
)

func TestNewCompressionWriterAutoX86(t *testing.T) {
	original := cpu.X86.HasAVX2
	defer func() { cpu.X86.HasAVX2 = original }()

	cpu.X86.HasAVX2 = true
	w, err := NewCompressionWriter(io.Discard, "auto", 1, 1)
	if err != nil {
		t.Fatalf("NewCompressionWriter with AVX2: %v", err)
	}
	if _, ok := w.(*zstd.Encoder); !ok {
		t.Fatalf("expected zstd writer when AVX2 is present")
	}
	w.Close()

	cpu.X86.HasAVX2 = false
	w, err = NewCompressionWriter(io.Discard, "auto", 1, 1)
	if err != nil {
		t.Fatalf("NewCompressionWriter without AVX2: %v", err)
	}
	if _, ok := w.(*lz4.Writer); !ok {
		t.Fatalf("expected lz4 writer when AVX2 is absent")
	}
	w.Close()
}
