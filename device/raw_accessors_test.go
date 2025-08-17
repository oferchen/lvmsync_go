//go:build linux

package device

import (
	"os"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestRawDeviceAccessorsAndClose(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "rawdev")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	d := &RawDevice{f: f, size: 123, blockSize: 456, logger: logger}
	if d.Path() != f.Name() {
		t.Fatalf("Path() = %q, want %q", d.Path(), f.Name())
	}
	if d.SizeBytes() != 123 {
		t.Fatalf("SizeBytes() = %d, want 123", d.SizeBytes())
	}
	if d.BlockSize() != 456 {
		t.Fatalf("BlockSize() = %d, want 456", d.BlockSize())
	}
	if err := d.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if logs.FilterMessage("raw_device_closed").Len() != 1 {
		t.Fatalf("expected raw_device_closed log")
	}
}

func TestIoctlGetUint64BadFD(t *testing.T) {
	if _, err := ioctlGetUint64(-1, 0); err == nil {
		t.Fatalf("expected error for bad fd")
	}
}
