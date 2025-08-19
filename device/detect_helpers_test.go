package device

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"lvmsync_go/internal/privilege"
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
	entries := logs.FilterMessage("detect_device_success").All()
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
	restore := lvm.SetEscalationChecker(func(string) error { return nil })
	defer restore()
	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	runner := NewRunner()
	runner.openLVMOverride = func(ctx context.Context, p string, _ *lvm.FDCache, _ string, _ *zap.Logger) (*LVMDevice, error) {
		return &LVMDevice{path: p, logger: zap.NewNop(), runner: runner}, nil
	}
	dev, err := detectLVMDevice(context.Background(), "/dev/test", "", runner, logger)
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if _, ok := dev.(*LVMDevice); !ok {
		t.Fatalf("expected LVMDevice, got %T", dev)
	}
	dev.Close()
	entries := logs.FilterMessage("detect_device_success").All()
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
	restore := lvm.SetEscalationChecker(func(string) error { return nil })
	defer restore()
	runner := NewRunner()
	runner.openLVMOverride = func(context.Context, string, *lvm.FDCache, string, *zap.Logger) (*LVMDevice, error) {
		return nil, errors.New("fail")
	}
	if _, err := detectLVMDevice(context.Background(), "/dev/test", "", runner, zap.NewNop()); err == nil {
		t.Fatalf("expected error")
	}
}

func TestDetectLVMDeviceEscalationError(t *testing.T) {
	restore := lvm.SetEscalationChecker(func(string) error { return errors.New("escalate fail") })
	defer restore()
	runner := NewRunner()
	called := false
	runner.openLVMOverride = func(context.Context, string, *lvm.FDCache, string, *zap.Logger) (*LVMDevice, error) {
		called = true
		return &LVMDevice{path: "/dev/test", logger: zap.NewNop(), runner: runner}, nil
	}
	if _, err := detectLVMDevice(context.Background(), "/dev/test", "sudo -n", runner, zap.NewNop()); err == nil {
		t.Fatalf("expected error")
	}
	if called {
		t.Fatalf("openLVM should not be called on escalation failure")
	}
}

func TestDetectRawDeviceSuccess(t *testing.T) {
	orig := openRawFunc
	openRawFunc = func(ctx context.Context, path string, offline bool, freezePath string, freezeArgs []string, thawPath string, thawArgs []string, freezeTimeout, thawTimeout time.Duration, esc privilege.Escalator, logger *zap.Logger, runner *Runner) (*RawDevice, error) {
		f, err := os.CreateTemp(t.TempDir(), "raw")
		if err != nil {
			return nil, err
		}
		return &RawDevice{f: f, logger: logger}, nil
	}
	defer func() { openRawFunc = orig }()
	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	ctx := WithForce(context.Background(), true)
	ctx = WithAllowOverwrite(ctx, true)
	ctx = WithYesIKnow(ctx, true)
	dev, err := detectRawDevice(ctx, "/dev/test", true, "", "", 0, 0, fakeEsc{}, logger, NewRunner())
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if _, ok := dev.(*RawDevice); !ok {
		t.Fatalf("expected RawDevice, got %T", dev)
	}
	dev.Close()
	entries := logs.FilterMessage("detect_device_success").All()
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
	ctx := WithForce(context.Background(), true)
	ctx = WithAllowOverwrite(ctx, true)
	ctx = WithYesIKnow(ctx, true)
	if _, err := detectRawDevice(ctx, "/dev/null", true, "", "", 0, 0, fakeEsc{}, zap.NewNop(), NewRunner()); err == nil {
		t.Fatalf("expected error")
	}
}

func TestDetectRawDeviceFreezeParseError(t *testing.T) {
	orig := openRawFunc
	called := false
	openRawFunc = func(ctx context.Context, path string, offline bool, freezePath string, freezeArgs []string, thawPath string, thawArgs []string, freezeTimeout, thawTimeout time.Duration, esc privilege.Escalator, logger *zap.Logger, runner *Runner) (*RawDevice, error) {
		called = true
		return nil, nil
	}
	defer func() { openRawFunc = orig }()
	core, logs := observer.New(zap.ErrorLevel)
	logger := zap.New(core)
	ctx := WithForce(context.Background(), true)
	ctx = WithAllowOverwrite(ctx, true)
	ctx = WithYesIKnow(ctx, true)
	_, err := detectRawDevice(ctx, "/dev/test", true, "/bin/echo \"unterminated", "", 0, 0, fakeEsc{}, logger, NewRunner())
	if err == nil || !strings.Contains(err.Error(), "invalid freeze command") {
		t.Fatalf("expected freeze parse error, got %v", err)
	}
	if called {
		t.Fatalf("openRawFunc should not be called on parse error")
	}
	entries := logs.FilterMessage("detect_device_failed").All()
	if len(entries) != 1 {
		t.Fatalf("expected error log, got %v", logs.All())
	}
	if entries[0].ContextMap()["device_type"] != "raw" || entries[0].ContextMap()["path"] != "/dev/test" {
		t.Fatalf("unexpected log fields: %v", entries[0].ContextMap())
	}
}

func TestDetectRawDeviceThawParseError(t *testing.T) {
	orig := openRawFunc
	called := false
	openRawFunc = func(ctx context.Context, path string, offline bool, freezePath string, freezeArgs []string, thawPath string, thawArgs []string, freezeTimeout, thawTimeout time.Duration, esc privilege.Escalator, logger *zap.Logger, runner *Runner) (*RawDevice, error) {
		called = true
		return nil, nil
	}
	defer func() { openRawFunc = orig }()
	core, logs := observer.New(zap.ErrorLevel)
	logger := zap.New(core)
	ctx := WithForce(context.Background(), true)
	ctx = WithAllowOverwrite(ctx, true)
	ctx = WithYesIKnow(ctx, true)
	_, err := detectRawDevice(ctx, "/dev/test", true, "", "/bin/echo \"unterminated", 0, 0, fakeEsc{}, logger, NewRunner())
	if err == nil || !strings.Contains(err.Error(), "invalid thaw command") {
		t.Fatalf("expected thaw parse error, got %v", err)
	}
	if called {
		t.Fatalf("openRawFunc should not be called on parse error")
	}
	entries := logs.FilterMessage("detect_device_failed").All()
	if len(entries) != 1 {
		t.Fatalf("expected error log, got %v", logs.All())
	}
	if entries[0].ContextMap()["device_type"] != "raw" || entries[0].ContextMap()["path"] != "/dev/test" {
		t.Fatalf("unexpected log fields: %v", entries[0].ContextMap())
	}
}
