package device

import (
	"context"
	"os"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestPrepareFreezeSuccess(t *testing.T) {
	issued, err := prepareFreeze(context.Background(), false, "true", nil, "true", nil, time.Second, zap.NewNop())
	if err != nil {
		t.Fatalf("prepareFreeze: %v", err)
	}
	if !issued {
		t.Fatalf("expected freeze to be issued")
	}
}

func TestPrepareFreezeFailure(t *testing.T) {
	if _, err := prepareFreeze(context.Background(), false, "false", nil, "true", nil, time.Second, zap.NewNop()); err == nil {
		t.Fatalf("expected freeze command failure")
	}
}

func TestOpenDeviceSuccess(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root")
	}
	loop, cleanup := setupLoop(t, 1<<20)
	defer cleanup()
	f, err := openDevice(loop)
	if err != nil {
		t.Fatalf("openDevice: %v", err)
	}
	f.Close()
}

func TestOpenDeviceFailure(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "file")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	f.Close()
	if _, err := openDevice(f.Name()); err == nil {
		t.Fatalf("expected error for non-block device")
	}
}

func TestQueryDeviceInfoSuccess(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root")
	}
	loop, cleanup := setupLoop(t, 1<<20)
	defer cleanup()
	f, err := openDevice(loop)
	if err != nil {
		t.Fatalf("openDevice: %v", err)
	}
	defer f.Close()
	size, bs, err := queryDeviceInfo(f, loop, zap.NewNop())
	if err != nil {
		t.Fatalf("queryDeviceInfo: %v", err)
	}
	if size == 0 || bs == 0 {
		t.Fatalf("expected non-zero size and block size")
	}
}

func TestQueryDeviceInfoFailure(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "file")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	defer f.Close()
	if _, _, err := queryDeviceInfo(f, f.Name(), zap.NewNop()); err == nil {
		t.Fatalf("expected error for non-block device")
	}
}
