package lvm

import (
	"context"
	"testing"
	"time"

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

func init() {
	SetEscalationCommand("")
}

func TestGetSnapshotUsage(t *testing.T) {
	orig := checkPrivs
	checkPrivs = func() error { return nil }
	t.Cleanup(func() { checkPrivs = orig })

	fb := &usageBackend{usage: 75.5}
	restore := SetBackend(fb)
	t.Cleanup(restore)

	usage, err := GetSnapshotUsage(context.Background(), "/dev/vg0/snap")
	if err != nil {
		t.Fatalf("GetSnapshotUsage failed: %v", err)
	}
	if usage != 75.5 {
		t.Fatalf("usage = %v, want 75.5", usage)
	}
}

func TestMonitorSnapshot(t *testing.T) {
	orig := checkPrivs
	checkPrivs = func() error { return nil }
	t.Cleanup(func() { checkPrivs = orig })

	fb := &usageBackend{usage: 90}
	restore := SetBackend(fb)
	t.Cleanup(restore)

	err := MonitorSnapshot("/dev/vg0/snap", 80, 10*time.Millisecond, make(chan struct{}))
	if err == nil {
		t.Fatalf("MonitorSnapshot expected error, got nil")
	}
}

func TestCheckDiskSpace(t *testing.T) {
	orig := statfsFunc
	statfsFunc = func(_ string, stat *unix.Statfs_t) error {
		stat.Bavail = 100
		stat.Bsize = 4096
		return nil
	}
	defer func() { statfsFunc = orig }()

	available, err := CheckDiskSpace("/mnt")
	if err != nil {
		t.Fatalf("CheckDiskSpace failed: %v", err)
	}
	const expected = 100 * 4096
	if available != expected {
		t.Fatalf("available = %d, want %d", available, expected)
	}
}
