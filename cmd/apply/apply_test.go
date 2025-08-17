package apply

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"lvmsync_go/device"
	"lvmsync_go/internal/config"
	digestpkg "lvmsync_go/internal/digest"
)

func TestRun(t *testing.T) {
	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig returned error: %v", err)
	}
	applyFile := "dumpfile"
	dest := "/dev/null"

	called := false
	r := NewRunner()
	r.applyFunc = func(c *config.Config, applyFileArg, destDevice string, _ *zap.Logger) error {
		called = true
		if applyFileArg != applyFile {
			t.Fatalf("expected applyFile %s, got %s", applyFile, applyFileArg)
		}
		if destDevice != dest {
			t.Fatalf("expected dest %s, got %s", dest, destDevice)
		}
		return nil
	}
	r.detectDevice = func(context.Context, string, bool, string, string, string, string, time.Duration, time.Duration, *zap.Logger, *device.Runner) (device.Device, error) {
		return &fakeDevice{path: dest}, nil
	}

	if err := r.Run(cfg, applyFile, []string{dest}, zap.NewNop()); err != nil {
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
	r := NewRunner()
	r.applyFunc = func(*config.Config, string, string, *zap.Logger) error { return nil }
	r.detectDevice = func(context.Context, string, bool, string, string, string, string, time.Duration, time.Duration, *zap.Logger, *device.Runner) (device.Device, error) {
		return &fakeDevice{path: dest}, nil
	}
	if err := r.Run(cfg, applyFile, []string{dest}, logger); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if core.count != 1 {
		t.Fatalf("expected Sync to be called once, got %d", core.count)
	}
}

func TestRunVerifyModes(t *testing.T) {
	data := make([]byte, 3<<20)
	destFile := t.TempDir() + "/dest"
	if err := os.WriteFile(destFile, data, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cases := []struct {
		name      string
		verify    string
		setDigest func(string) string
		wantErr   bool
	}{
		{
			name:   "full",
			verify: "full",
			setDigest: func(path string) string {
				sum, err := digestpkg.SumFile(path, digestpkg.SHA256)
				if err != nil {
					t.Fatalf("SumFile: %v", err)
				}
				return fmt.Sprintf("%x", sum[:])
			},
		},
		{
			name:   "sampled",
			verify: "sampled",
			setDigest: func(path string) string {
				sum, err := digestpkg.SampledSumFile(path, digestpkg.SHA256)
				if err != nil {
					t.Fatalf("SampledSumFile: %v", err)
				}
				return fmt.Sprintf("%x", sum[:])
			},
		},
		{
			name:      "none",
			verify:    "none",
			setDigest: func(string) string { return "" },
		},
		{
			name:      "mismatch",
			verify:    "full",
			setDigest: func(string) string { return strings.Repeat("0", 64) },
			wantErr:   true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := config.DefaultConfig()
			if err != nil {
				t.Fatalf("DefaultConfig returned error: %v", err)
			}
			cfg.VerifyLevel = tc.verify
			cfg.ChecksumAlgorithm = digestpkg.SHA256
			if d := tc.setDigest(destFile); d != "" {
				t.Setenv("LVMSYNC_SOURCE_DIGEST", d)
			}
			r := NewRunner()
			r.applyFunc = func(*config.Config, string, string, *zap.Logger) error { return nil }
			r.detectDevice = func(context.Context, string, bool, string, string, string, string, time.Duration, time.Duration, *zap.Logger, *device.Runner) (device.Device, error) {
				return &fakeDevice{path: destFile}, nil
			}
			err = r.Run(cfg, "-", []string{destFile}, zap.NewNop())
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
			} else if err != nil {
				t.Fatalf("Run returned error: %v", err)
			}
		})
	}
}

type fakeDevice struct{ path string }

func (f *fakeDevice) Path() string                                            { return f.path }
func (f *fakeDevice) SizeBytes() uint64                                       { return 0 }
func (f *fakeDevice) BlockSize() uint64                                       { return 0 }
func (f *fakeDevice) Snapshot(context.Context, string) (device.Device, error) { return f, nil }
func (f *fakeDevice) Cleanup(context.Context) error                           { return nil }
func (f *fakeDevice) Close() error                                            { return nil }
