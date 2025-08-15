package transfer

import (
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
