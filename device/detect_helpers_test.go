package device

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"lvmsync_go/lvm"
)

func TestDetectFileDeviceSuccess(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "file")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	f.Close()
	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	dev, err := detectFileDevice(f.Name(), logger)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if _, ok := dev.(*FileDevice); !ok {
		t.Fatalf("expected FileDevice, got %T", dev)
	}
	dev.Close()
	entries := logs.FilterMessage("detect device success").All()
	found := false
	for _, e := range entries {
		if e.ContextMap()["device_type"] == "file" && e.ContextMap()["path"] == f.Name() {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected success log")
	}
}

func TestDetectFileDeviceError(t *testing.T) {
	dir := t.TempDir()
	if _, err := detectFileDevice(dir, zap.NewNop()); err == nil {
		t.Fatalf("expected error")
	}
}

func TestDetectLVMDeviceSuccess(t *testing.T) {
	orig := openLVMFunc
	openLVMFunc = func(p string, _ *lvm.FDCache, _ string, _ *zap.Logger) (*LVMDevice, error) {
		return &LVMDevice{path: p, logger: zap.NewNop()}, nil
	}
	defer func() { openLVMFunc = orig }()
	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	dev, err := detectLVMDevice("/dev/test", "", logger)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if _, ok := dev.(*LVMDevice); !ok {
		t.Fatalf("expected LVMDevice, got %T", dev)
	}
	dev.Close()
	entries := logs.FilterMessage("detect device success").All()
	found := false
	for _, e := range entries {
		if e.ContextMap()["device_type"] == "lvm" && e.ContextMap()["path"] == "/dev/test" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected success log")
	}
}

func TestDetectLVMDeviceError(t *testing.T) {
	orig := openLVMFunc
	openLVMFunc = func(string, *lvm.FDCache, string, *zap.Logger) (*LVMDevice, error) {
		return nil, errors.New("fail")
	}
	defer func() { openLVMFunc = orig }()
	if _, err := detectLVMDevice("/dev/test", "", zap.NewNop()); err == nil {
		t.Fatalf("expected error")
	}
}

func TestDetectLVMDeviceEscalationError(t *testing.T) {
	restore := lvm.SetEscalationChecker(func(string) error { return errors.New("escalate fail") })
	defer restore()
	orig := openLVMFunc
	called := false
	openLVMFunc = func(string, *lvm.FDCache, string, *zap.Logger) (*LVMDevice, error) {
		called = true
		return &LVMDevice{path: "/dev/test", logger: zap.NewNop()}, nil
	}
	defer func() { openLVMFunc = orig }()
	if _, err := detectLVMDevice("/dev/test", "sudo -n", zap.NewNop()); err == nil {
		t.Fatalf("expected error")
	}
	if called {
		t.Fatalf("openLVMFunc should not be called on escalation failure")
	}
}

func TestDetectRawDeviceSuccess(t *testing.T) {
	orig := openRawFunc
	openRawFunc = func(ctx context.Context, path string, offline bool, freezePath string, freezeArgs []string, thawPath string, thawArgs []string, freezeTimeout, thawTimeout time.Duration, logger *zap.Logger) (*RawDevice, error) {
		f, err := os.CreateTemp(t.TempDir(), "raw")
		if err != nil {
			return nil, err
		}
		return &RawDevice{f: f, logger: logger}, nil
	}
	defer func() { openRawFunc = orig }()
	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	dev, err := detectRawDevice(context.Background(), "/dev/test", true, "", "", 0, 0, logger)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if _, ok := dev.(*RawDevice); !ok {
		t.Fatalf("expected RawDevice, got %T", dev)
	}
	dev.Close()
	entries := logs.FilterMessage("detect device success").All()
	found := false
	for _, e := range entries {
		if e.ContextMap()["device_type"] == "raw" && e.ContextMap()["path"] == "/dev/test" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected success log")
	}
}

func TestDetectRawDeviceError(t *testing.T) {
	if _, err := detectRawDevice(context.Background(), "/dev/null", true, "", "", 0, 0, zap.NewNop()); err == nil {
		t.Fatalf("expected error")
	}
}
