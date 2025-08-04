//go:build arm64

package transfer

import (
	"io"
	"testing"

	"golang.org/x/sys/cpu"
)

func TestNewCompressionWriterAutoARM64(t *testing.T) {
	original := cpu.ARM64.HasASIMD
	defer func() { cpu.ARM64.HasASIMD = original }()

	cpu.ARM64.HasASIMD = true
	w, err := NewCompressionWriter(io.Discard, "auto", 1)
	if err != nil {
		t.Fatalf("NewCompressionWriter with ASIMD: %v", err)
	}
	if _, ok := w.(*zstdWriteCloser); !ok {
		t.Fatalf("expected zstd writer when ASIMD is present")
	}
	w.Close()

	cpu.ARM64.HasASIMD = false
	w, err = NewCompressionWriter(io.Discard, "auto", 1)
	if err != nil {
		t.Fatalf("NewCompressionWriter without ASIMD: %v", err)
	}
	if _, ok := w.(*lz4WriteCloser); !ok {
		t.Fatalf("expected lz4 writer when ASIMD is absent")
	}
	w.Close()
}
