package lvm

import "context"

type lvmBackend interface {
	CreateSnapshot(ctx context.Context, lvPath, snapshotName, size string) error
	RemoveSnapshot(ctx context.Context, snapshotPath string) error
	GetSnapshotUsage(ctx context.Context, snapshotPath string) (float64, error)
	GetVolumeGroupFreeSpace(ctx context.Context, vgName string) (uint64, error)
	ListVolumeGroups(ctx context.Context, candidates []string) ([]VolumeGroup, error)
}
