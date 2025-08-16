package transfer

import (
	"bytes"
	"os"
	"testing"
)

func TestLoopDeviceSetup(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root")
	}
	f, err := os.CreateTemp(t.TempDir(), "loop-*.img")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	f.Close()
	loop := setupLoop(t, f.Name())
	if loop == "" {
		t.Fatalf("expected loop device path")
	}
}

func TestLoopDeviceReadWrite(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root")
	}
	f, err := os.CreateTemp(t.TempDir(), "loop-*.img")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	if err := f.Truncate(4096); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	f.Close()
	loop := setupLoop(t, f.Name())
	if loop == "" {
		t.Fatalf("expected loop device path")
	}
	dev, err := os.OpenFile(loop, os.O_RDWR, 0)
	if err != nil {
		t.Fatalf("open loop: %v", err)
	}
	data := []byte{1, 2, 3, 4}
	if _, err := dev.WriteAt(data, 0); err != nil {
		t.Fatalf("write loop: %v", err)
	}
	dev.Close()
	content, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatalf("read backing: %v", err)
	}
	if len(content) < len(data) || !bytes.Equal(content[:len(data)], data) {
		t.Fatalf("backing file mismatch")
	}
}
