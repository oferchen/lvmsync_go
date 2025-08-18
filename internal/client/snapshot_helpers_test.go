package client

import (
	"context"
	"errors"
	"math"
	"testing"

	"lvmsync_go/internal/config"
	"lvmsync_go/lvm"

	"go.uber.org/zap"
)

func TestCalculateSnapshotSize(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		cfg := &config.Config{SnapshotSize: "1024"}
		r := NewRunnerWithDeps(func(string, string, *lvm.FDCache, *zap.Logger) (uint64, error) { return 1024, nil }, nil, nil, nil, nil, nil, nil, nil, nil, nil)

		cache := lvm.NewDeviceFDCache(zap.NewNop())
		defer cache.Close()
		size, err := r.calculateSnapshotSize(cfg, "/dev/vg/lv", cache, zap.NewNop())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if size != 1024 {
			t.Fatalf("expected 1024, got %d", size)
		}
	})

	t.Run("invalid", func(t *testing.T) {
		cfg := &config.Config{SnapshotSize: "bad"}
		r := NewRunnerWithDeps(func(string, string, *lvm.FDCache, *zap.Logger) (uint64, error) { return 0, errors.New("bad") }, nil, nil, nil, nil, nil, nil, nil, nil, nil)

		cache := lvm.NewDeviceFDCache(zap.NewNop())
		defer cache.Close()
		if _, err := r.calculateSnapshotSize(cfg, "/dev/vg/lv", cache, zap.NewNop()); err == nil {
			t.Fatalf("expected error for invalid size")
		}
	})
}

func TestEnsureVolumeGroups(t *testing.T) {
	logger := zap.NewNop()
	cache := lvm.NewDeviceFDCache(logger)
	defer cache.Close()

	t.Run("sets missing volume group", func(t *testing.T) {
		cfg := &config.Config{}
		r := NewRunner()
		if err := r.ensureVolumeGroups(context.Background(), cfg, "/dev/sourcevg/lv1", cache, logger); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.VolumeGroup != "sourcevg" {
			t.Fatalf("expected volume group 'sourcevg', got %q", cfg.VolumeGroup)
		}
	})

	t.Run("preserves existing volume group", func(t *testing.T) {
		cfg := &config.Config{VolumeGroup: "existing"}
		r := NewRunner()
		if err := r.ensureVolumeGroups(context.Background(), cfg, "/dev/othervg/lv", cache, logger); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.VolumeGroup != "existing" {
			t.Fatalf("volume group changed to %q", cfg.VolumeGroup)
		}
	})
}

func TestCheckDiskSpaceForSnapshot(t *testing.T) {
	logger := zap.NewNop()

	t.Run("sufficient", func(t *testing.T) {
		cfg := &config.Config{SkipDiskCheck: false}
		r := NewRunner()
		if err := r.checkDiskSpaceForSnapshot(cfg, 1, logger); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("insufficient", func(t *testing.T) {
		cfg := &config.Config{SkipDiskCheck: false}
		r := NewRunner()
		if err := r.checkDiskSpaceForSnapshot(cfg, math.MaxUint64, logger); err == nil {
			t.Fatalf("expected error for insufficient space")
		}
	})
}
