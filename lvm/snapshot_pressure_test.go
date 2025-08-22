package lvm_test

import (
	"context"
	"strings"
	"testing"
	"time"

	rootcmd "github.com/oferchen/lvmsync_go/cmd/root"
	"github.com/oferchen/lvmsync_go/internal/client"
	"github.com/oferchen/lvmsync_go/internal/config"
	"github.com/oferchen/lvmsync_go/internal/exitcode"
	lvm "github.com/oferchen/lvmsync_go/lvm"

	"go.uber.org/zap"
)

type pressureBackend struct{ usage float64 }

func (b *pressureBackend) CreateSnapshot(context.Context, string, string, string) error { return nil }
func (b *pressureBackend) RemoveSnapshot(context.Context, string) error                 { return nil }
func (b *pressureBackend) GetSnapshotUsage(context.Context, string) (float64, error) {
	return b.usage, nil
}
func (b *pressureBackend) GetVolumeGroupFreeSpace(context.Context, string) (uint64, error) {
	return 0, nil
}
func (b *pressureBackend) ListVolumeGroups(context.Context, []string) ([]lvm.VolumeGroup, error) {
	return nil, nil
}

func (b *pressureBackend) CreateLogicalVolume(context.Context, string, string, uint64) error {
	return nil
}

func TestSnapshotPressureAbortCleanup(t *testing.T) {
	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig error: %v", err)
	}
	cfg.SkipDiskCheck = true
	cfg.VolumeGroup = "vg"
	cfg.TargetVolumeGroup = "vg2"
	cfg.SnapshotSize = "1G"

	removed := false
	lRunner := lvm.NewRunnerWithDeps(nil, func() error { return nil }, nil, &pressureBackend{usage: 90}, "")
	monitor := func(ctx context.Context, path string, threshold float64, _ time.Duration, logger *zap.Logger) error {
		return lRunner.MonitorSnapshot(ctx, path, threshold, 10*time.Millisecond, logger)
	}
	r := client.NewRunnerWithDeps(
		func(string, string, *lvm.FDCache, *zap.Logger) (uint64, error) { return 1024, nil },
		nil, nil, nil, nil,
		func(context.Context, string, string, string, *zap.Logger) error { return nil },
		func(name, vg string, _ *zap.Logger) string { return "/dev/" + vg + "/" + name },
		monitor,
		func(context.Context, string, *zap.Logger) error { removed = true; return nil },
		nil,
	)

	logger := zap.NewNop()
	snap, monitorCh, cleanup, err := r.PrepareSnapshot(context.Background(), cfg, "/dev/vg/origin", logger)
	if err != nil {
		t.Fatalf("PrepareSnapshot: %v", err)
	}

	err = client.ExecuteClient(context.Background(), func(context.Context, string, string) error { return nil }, snap, "/dev/vg/target", nil, monitorCh, logger)
	if err == nil || !strings.Contains(err.Error(), "snapshot exhausted") {
		t.Fatalf("expected snapshot exhausted error, got %v", err)
	}
	if code := rootcmd.ExitCode(err); code != exitcode.ErrSnapshotExhausted {
		t.Fatalf("exit code = %d; want %d", code, exitcode.ErrSnapshotExhausted)
	}

	cleanup()
	if !removed {
		t.Fatalf("snapshot not removed")
	}
}
