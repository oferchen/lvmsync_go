package manifest

import (
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"lvmsync_go/device"
	"lvmsync_go/internal/config"
	manifestpkg "lvmsync_go/manifest"
)

type syncTrackerCore struct {
	zapcore.Core
	syncs *int
}

func (s syncTrackerCore) Sync() error {
	*s.syncs++
	_ = s.Core.Sync()
	return fmt.Errorf("sync error")
}

func TestRunDefaultOutputPath(t *testing.T) {
	dir := t.TempDir()
	devicePath := filepath.Join(dir, "dev.img")
	if err := os.WriteFile(devicePath, []byte("data"), 0o600); err != nil {
		t.Fatalf("write device: %v", err)
	}

	info := device.NewInfoWithDeps(func(context.Context, string) (string, error) { return "uuid", nil }, nil, nil, nil, nil)

	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	cfg.ManifestAllowMounted = true

	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	r := NewRunnerWithDeps(func(ctx context.Context, dev, output string, logger *zap.Logger, interval time.Duration, allow bool, cdcMin, cdcAvg, cdcMax, hybrid uint32, opts ...manifestpkg.IndexOption) error {
		opts = append(opts, manifestpkg.WithDeviceInfo(info))
		return manifestpkg.Rebuild(ctx, dev, output, logger, interval, allow, cdcMin, cdcAvg, cdcMax, hybrid, opts...)
	}, nil, nil, manifestpkg.Open)
	if err := r.Run(cfg, []string{"rebuild", devicePath}, logger); err != nil {
		t.Fatalf("Run: %v", err)
	}

	outPath := devicePath + ".manifest"
	if _, err := os.Stat(outPath); err != nil {
		t.Fatalf("manifest not created at %s: %v", outPath, err)
	}

	rebuildLogs := logs.FilterMessage("rebuilding manifest").All()
	if len(rebuildLogs) != 1 {
		t.Fatalf("expected 1 rebuilding log, got %d", len(rebuildLogs))
	}
	ctx := rebuildLogs[0].ContextMap()
	if ctx["device"] != devicePath || ctx["output"] != outPath {
		t.Fatalf("unexpected log fields: %v", ctx)
	}
	completeLogs := logs.FilterMessage("rebuild complete").All()
	if len(completeLogs) != 1 {
		t.Fatalf("expected 1 complete log, got %d", len(completeLogs))
	}
}

func TestRunManifestPathFlag(t *testing.T) {
	dir := t.TempDir()
	devicePath := filepath.Join(dir, "dev.img")
	if err := os.WriteFile(devicePath, []byte("data"), 0o600); err != nil {
		t.Fatalf("write device: %v", err)
	}

	info := device.NewInfoWithDeps(func(context.Context, string) (string, error) { return "uuid", nil }, nil, nil, nil, nil)

	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	cfg.ManifestAllowMounted = true

	outputPath := filepath.Join(dir, "custom.manifest")
	args := []string{"rebuild", "--manifest-path", outputPath, devicePath}
	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	r := NewRunnerWithDeps(func(ctx context.Context, dev, output string, logger *zap.Logger, interval time.Duration, allow bool, cdcMin, cdcAvg, cdcMax, hybrid uint32, opts ...manifestpkg.IndexOption) error {
		opts = append(opts, manifestpkg.WithDeviceInfo(info))
		return manifestpkg.Rebuild(ctx, dev, output, logger, interval, allow, cdcMin, cdcAvg, cdcMax, hybrid, opts...)
	}, nil, nil, manifestpkg.Open)
	if err := r.Run(cfg, args, logger); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if _, err := os.Stat(outputPath); err != nil {
		t.Fatalf("manifest not created at %s: %v", outputPath, err)
	}
	if _, err := os.Stat(devicePath + ".manifest"); !os.IsNotExist(err) {
		t.Fatalf("unexpected manifest at default path")
	}

	rebuildLogs := logs.FilterMessage("rebuilding manifest").All()
	if len(rebuildLogs) != 1 {
		t.Fatalf("expected 1 rebuilding log, got %d", len(rebuildLogs))
	}
	ctx := rebuildLogs[0].ContextMap()
	if ctx["device"] != devicePath || ctx["output"] != outputPath {
		t.Fatalf("unexpected log fields: %v", ctx)
	}
	completeLogs := logs.FilterMessage("rebuild complete").All()
	if len(completeLogs) != 1 {
		t.Fatalf("expected 1 complete log, got %d", len(completeLogs))
	}
}

func TestRunLogsConfigWarnings(t *testing.T) {
	dir := t.TempDir()
	devicePath := filepath.Join(dir, "dev.img")
	if err := os.WriteFile(devicePath, []byte("data"), 0o600); err != nil {
		t.Fatalf("write device: %v", err)
	}

	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	cfg.ManifestAllowMounted = true
	cfg.DryRun = true

	cfgPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("bogus: 1\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	args := []string{"rebuild", "--config", cfgPath, devicePath}
	logger := zap.NewNop()
	if err := Run(cfg, args, logger); err == nil {
		t.Fatalf("expected error for unknown key")
	}
}

func TestRunMissingArgs(t *testing.T) {
	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}

	cases := []struct {
		name string
		args []string
	}{
		{"missing subcommand", []string{}},
		{"missing device", []string{"rebuild"}},
		{"gc missing path", []string{"gc"}},
		{"compact missing path", []string{"compact"}},
		{"check missing path", []string{"check"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var runErr error
			defer func() {
				if r := recover(); r == nil && runErr == nil {
					t.Fatalf("expected failure for args %v", tc.args)
				}
			}()
			runErr = Run(cfg, tc.args, zap.NewNop())
			if runErr == nil {
				t.Fatalf("expected failure for args %v", tc.args)
			}
		})
	}
}

func TestRunNilLoggerPanics(t *testing.T) {
	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	r := NewRunner()
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("expected panic")
		}
	}()
	_ = r.Run(cfg, []string{"rebuild", "/dev/test"}, nil)
}

func TestRunAppliesManifestTimeout(t *testing.T) {
	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	cfg.ManifestTimeout = 2 * time.Second
	var captured context.Context
	r := NewRunnerWithDeps(func(ctx context.Context, _, _ string, _ *zap.Logger, _ time.Duration, allow bool, cdcMin, cdcAvg, cdcMax, hybrid uint32, opts ...manifestpkg.IndexOption) error {
		captured = ctx
		return nil
	}, nil, nil, manifestpkg.Open)
	if err := r.Run(cfg, []string{"rebuild", "/dev/test"}, zap.NewNop()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if _, ok := captured.Deadline(); !ok {
		t.Fatalf("expected context with deadline")
	}
}

func TestRunZeroManifestTimeoutUsesBackground(t *testing.T) {
	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	cfg.ManifestAllowMounted = true
	cfg.ManifestTimeout = 0
	var captured context.Context
	r := NewRunnerWithDeps(func(ctx context.Context, _, _ string, _ *zap.Logger, _ time.Duration, allow bool, cdcMin, cdcAvg, cdcMax, hybrid uint32, opts ...manifestpkg.IndexOption) error {
		captured = ctx
		return nil
	}, nil, nil, manifestpkg.Open)
	if err := r.Run(cfg, []string{"rebuild", "/dev/test"}, zap.NewNop()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if _, ok := captured.Deadline(); ok {
		t.Fatalf("unexpected deadline on context")
	}
}

func TestRunContextNoDeadline(t *testing.T) {
	dir := t.TempDir()
	devicePath := filepath.Join(dir, "dev.img")
	if err := os.WriteFile(devicePath, []byte("data"), 0o600); err != nil {
		t.Fatalf("write device: %v", err)
	}

	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	cfg.ManifestTimeout = 0
	var captured context.Context
	r := NewRunnerWithDeps(func(ctx context.Context, _, _ string, _ *zap.Logger, _ time.Duration, allow bool, cdcMin, cdcAvg, cdcMax, hybrid uint32, opts ...manifestpkg.IndexOption) error {
		captured = ctx
		return nil
	}, nil, nil, manifestpkg.Open)
	if err := r.Run(cfg, []string{"rebuild", "/dev/test"}, zap.NewNop()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if _, ok := captured.Deadline(); ok {
		t.Fatalf("unexpected deadline on context")
	}

}

func TestRunSyncsLoggerDryRun(t *testing.T) {
	dir := t.TempDir()
	devicePath := filepath.Join(dir, "dev.img")
	if err := os.WriteFile(devicePath, []byte("data"), 0o600); err != nil {
		t.Fatalf("write device: %v", err)
	}

	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}

	cfg.ManifestAllowMounted = true
	cfg.DryRun = true

	var syncs int
	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(syncTrackerCore{Core: core, syncs: &syncs})
	if err := Run(cfg, []string{"rebuild", devicePath}, logger); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if syncs != 1 {
		t.Fatalf("expected logger.Sync called once, got %d", syncs)
	}
	if logs.FilterMessage("logger_sync_error").Len() != 1 {
		t.Fatalf("expected sync error log")
	}
}

func TestRunWritesVersion(t *testing.T) {
	dir := t.TempDir()
	devicePath := filepath.Join(dir, "dev.img")
	if err := os.WriteFile(devicePath, []byte("data"), 0o600); err != nil {
		t.Fatalf("write device: %v", err)
	}
	info := device.NewInfoWithDeps(func(context.Context, string) (string, error) { return "uuid", nil }, nil, nil, nil, nil)
	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	cfg.ManifestAllowMounted = true
	r := NewRunnerWithDeps(func(ctx context.Context, dev, output string, logger *zap.Logger, interval time.Duration, allow bool, cdcMin, cdcAvg, cdcMax, hybrid uint32, opts ...manifestpkg.IndexOption) error {
		opts = append(opts, manifestpkg.WithDeviceInfo(info))
		return manifestpkg.Rebuild(ctx, dev, output, logger, interval, allow, cdcMin, cdcAvg, cdcMax, hybrid, opts...)
	}, nil, nil, manifestpkg.Open)
	if err := r.Run(cfg, []string{"rebuild", devicePath}, zap.NewNop()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	data, err := os.ReadFile(devicePath + ".manifest")
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if v := binary.LittleEndian.Uint32(data[:4]); v != manifestpkg.Version {
		t.Fatalf("version mismatch: got %d want %d", v, manifestpkg.Version)
	}
}
