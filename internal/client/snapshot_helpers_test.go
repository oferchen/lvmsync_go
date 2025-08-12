package client

import (
	"context"
	"errors"
	"math"
	"testing"

	"lvmsync_go/config"

	"go.uber.org/zap"
)

func TestCalculateSnapshotSize(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		cfg := &config.Config{SnapshotSize: "1024"}
		origParse := parseSnapshotSize
		parseSnapshotSize = func(string, string) (uint64, error) { return 1024, nil }
		defer func() { parseSnapshotSize = origParse }()

		size, err := calculateSnapshotSize(cfg, "/dev/vg/lv")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if size != 1024 {
			t.Fatalf("expected 1024, got %d", size)
		}
	})

	t.Run("invalid", func(t *testing.T) {
		cfg := &config.Config{SnapshotSize: "bad"}
		origParse := parseSnapshotSize
		parseSnapshotSize = func(string, string) (uint64, error) { return 0, errors.New("bad") }
		defer func() { parseSnapshotSize = origParse }()

		if _, err := calculateSnapshotSize(cfg, "/dev/vg/lv"); err == nil {
			t.Fatalf("expected error for invalid size")
		}
	})
}

func TestEnsureVolumeGroups(t *testing.T) {
	logger := zap.NewNop()

	t.Run("sets missing volume group", func(t *testing.T) {
		cfg := &config.Config{}
		if err := ensureVolumeGroups(context.Background(), cfg, "/dev/sourcevg/lv1", logger); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.VolumeGroup != "sourcevg" {
			t.Fatalf("expected volume group 'sourcevg', got %q", cfg.VolumeGroup)
		}
	})

	t.Run("preserves existing volume group", func(t *testing.T) {
		cfg := &config.Config{VolumeGroup: "existing"}
		if err := ensureVolumeGroups(context.Background(), cfg, "/dev/othervg/lv", logger); err != nil {
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
		if err := checkDiskSpaceForSnapshot(cfg, 1, logger); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("insufficient", func(t *testing.T) {
		cfg := &config.Config{SkipDiskCheck: false}
		if err := checkDiskSpaceForSnapshot(cfg, math.MaxUint64, logger); err == nil {
			t.Fatalf("expected error for insufficient space")
		}
	})
}
