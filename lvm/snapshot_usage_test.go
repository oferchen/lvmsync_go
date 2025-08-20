package lvm

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"
	"golang.org/x/sys/unix"
)

type usageBackend struct {
	usage float64
}

func (f *usageBackend) CreateSnapshot(context.Context, string, string, string) error { return nil }
func (f *usageBackend) RemoveSnapshot(context.Context, string) error                 { return nil }
func (f *usageBackend) GetSnapshotUsage(context.Context, string) (float64, error) {
	return f.usage, nil
}
func (f *usageBackend) GetVolumeGroupFreeSpace(context.Context, string) (uint64, error) {
	return 0, nil
}
func (f *usageBackend) ListVolumeGroups(context.Context, []string) ([]VolumeGroup, error) {
	return nil, nil
}

func (f *usageBackend) CreateLogicalVolume(context.Context, string, string, uint64) error {
	return nil
}

func TestGetSnapshotUsage(t *testing.T) {
	fb := &usageBackend{usage: 75.5}
	r := NewRunnerWithDeps(nil, func() error { return nil }, nil, fb, "")
	usage, err := r.GetSnapshotUsage(context.Background(), "/dev/vg0/snap", zap.NewNop())
	if err != nil {
		t.Fatalf("GetSnapshotUsage failed: %v", err)
	}
	if usage != 75.5 {
		t.Fatalf("usage = %v, want 75.5", usage)
	}
}

func TestMonitorSnapshot(t *testing.T) {
	fb := &usageBackend{usage: 90}
	r := NewRunnerWithDeps(nil, func() error { return nil }, nil, fb, "")
	err := r.MonitorSnapshot(context.Background(), "/dev/vg0/snap", 80, 10*time.Millisecond, zap.NewNop())
	if err == nil {
		t.Fatalf("MonitorSnapshot expected error, got nil")
	}
}

func TestCheckDiskSpace(t *testing.T) {
	r := NewRunnerWithDeps(func(_ string, stat *unix.Statfs_t) error {
		stat.Bavail = 100
		stat.Bsize = 4096
		return nil
	}, nil, nil, nil, "")
	available, err := r.CheckDiskSpace("/mnt", zap.NewNop())
	if err != nil {
		t.Fatalf("CheckDiskSpace failed: %v", err)
	}
	const expected = 100 * 4096
	if available != expected {
		t.Fatalf("available = %d, want %d", available, expected)
	}
}
