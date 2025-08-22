package client

import (
	"context"
	"errors"
	"math"
	"testing"

	"lvmsync_go/internal/config"
	"lvmsync_go/lvm"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestCalculateSnapshotSize(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		cfg := &config.Config{SnapshotSize: "1024"}
		r := NewRunnerWithDeps(func(string, string, *lvm.FDCache, *zap.Logger) (uint64, error) { return 1024, nil }, nil, nil, nil, nil, nil, nil, nil, nil, nil)

		cache, err := lvm.NewDeviceFDCache(zap.NewNop())
		if err != nil {
			t.Fatalf("NewDeviceFDCache: %v", err)
		}
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

		cache, err := lvm.NewDeviceFDCache(zap.NewNop())
		if err != nil {
			t.Fatalf("NewDeviceFDCache: %v", err)
		}
		defer cache.Close()
		if _, err := r.calculateSnapshotSize(cfg, "/dev/vg/lv", cache, zap.NewNop()); err == nil {
			t.Fatalf("expected error for invalid size")
		}
	})
}

func TestEnsureVolumeGroups(t *testing.T) {
	logger := zap.NewNop()
	cache, err := lvm.NewDeviceFDCache(logger)
	if err != nil {
		t.Fatalf("NewDeviceFDCache: %v", err)
	}
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

	t.Run("selects target volume group", func(t *testing.T) {
		core, obs := observer.New(zap.InfoLevel)
		logger := zap.New(core)
		cache, err := lvm.NewDeviceFDCache(logger)
		if err != nil {
			t.Fatalf("NewDeviceFDCache: %v", err)
		}
		defer cache.Close()
		cfg := &config.Config{VolumeGroup: "src", TargetVGCandidates: []string{"cand1", "cand2"}}
		r := NewRunnerWithDeps(nil, nil,
			func(string, *lvm.FDCache, *zap.Logger) (uint64, error) { return 100, nil },
			func(context.Context, []string, uint64) (lvm.VolumeGroup, error) {
				return lvm.VolumeGroup{Name: "cand2"}, nil
			},
			nil, nil, nil, nil, nil, nil)
		if err := r.ensureVolumeGroups(context.Background(), cfg, "/dev/src/lv", cache, logger); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.TargetVolumeGroup != "cand2" {
			t.Fatalf("expected target volume group 'cand2', got %q", cfg.TargetVolumeGroup)
		}
		found := false
		for _, entry := range obs.All() {
			if entry.Message == "Selected target volume group" {
				if entry.ContextMap()["target_volume_group"] != "cand2" {
					t.Fatalf("unexpected log field %v", entry.ContextMap()["target_volume_group"])
				}
				found = true
			}
		}
		if !found {
			t.Fatalf("expected log for selected target volume group")
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
