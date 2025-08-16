package apply

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"lvmsync_go/device"
	"lvmsync_go/internal/config"
)

func TestRun(t *testing.T) {
	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig returned error: %v", err)
	}
	applyFile := "dumpfile"
	dest := "/dev/null"

	called := false
	origApply := applyFunc
	origDetect := detectDevice
	applyFunc = func(c *config.Config, applyFileArg, destDevice string, _ *zap.Logger) error {
		called = true
		if applyFileArg != applyFile {
			t.Fatalf("expected applyFile %s, got %s", applyFile, applyFileArg)
		}
		if destDevice != dest {
			t.Fatalf("expected dest %s, got %s", dest, destDevice)
		}
		return nil
	}
	detectDevice = func(context.Context, string, bool, string, string, string, string, time.Duration, time.Duration, *zap.Logger) (device.Device, error) {
		return &fakeDevice{path: dest}, nil
	}
	defer func() { applyFunc = origApply; detectDevice = origDetect }()

	if err := Run(cfg, applyFile, []string{dest}, zap.NewNop()); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !called {
		t.Fatalf("applyFunc was not called")
	}
}

type countingSyncCore struct {
	zapcore.Core
	count int
}

func (c *countingSyncCore) Sync() error {
	c.count++
	return nil
}

func TestRunSyncsLogger(t *testing.T) {
	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig returned error: %v", err)
	}
	core := &countingSyncCore{Core: zapcore.NewNopCore()}
	logger := zap.New(core)
	applyFile := "dumpfile"
	dest := "/dev/null"
	origApply := applyFunc
	origDetect := detectDevice
	applyFunc = func(*config.Config, string, string, *zap.Logger) error { return nil }
	detectDevice = func(context.Context, string, bool, string, string, string, string, time.Duration, time.Duration, *zap.Logger) (device.Device, error) {
		return &fakeDevice{path: dest}, nil
	}
	defer func() { applyFunc = origApply; detectDevice = origDetect }()
	if err := Run(cfg, applyFile, []string{dest}, logger); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if core.count != 1 {
		t.Fatalf("expected Sync to be called once, got %d", core.count)
	}
}

type fakeDevice struct{ path string }

func (f *fakeDevice) Path() string                                            { return f.path }
func (f *fakeDevice) SizeBytes() uint64                                       { return 0 }
func (f *fakeDevice) BlockSize() uint64                                       { return 0 }
func (f *fakeDevice) Snapshot(context.Context, string) (device.Device, error) { return f, nil }
func (f *fakeDevice) Cleanup(context.Context) error                           { return nil }
func (f *fakeDevice) Close() error                                            { return nil }
