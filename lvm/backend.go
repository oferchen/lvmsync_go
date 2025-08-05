package lvm

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/dpeckett/lvm2"
)

type lvmBackend interface {
	CreateSnapshot(lvPath, snapshotName, size string) error
	RemoveSnapshot(snapshotPath string) error
	GetSnapshotUsage(snapshotPath string) (float64, error)
	GetVolumeGroupFreeSpace(vgName string) (uint64, error)
	ListVolumeGroups() ([]VolumeGroup, error)
}

type lvm2Backend struct {
	client *lvm2.Client
}

func newLVMBackend() lvmBackend {
	return &lvm2Backend{client: lvm2.NewClient()}
}

func (b *lvm2Backend) CreateSnapshot(lvPath, snapshotName, size string) error {
	ctx := context.Background()
	opts := lvm2.CreateLVOptions{
		Name:     snapshotName,
		VGName:   lvPath,
		Size:     size,
		Snapshot: true,
	}
	return b.client.CreateLogicalVolume(ctx, opts)
}

func (b *lvm2Backend) RemoveSnapshot(snapshotPath string) error {
	ctx := context.Background()
	opts := lvm2.RemoveLVOptions{
		Name:  snapshotPath,
		Force: true,
	}
	return b.client.RemoveLogicalVolume(ctx, opts)
}

func (b *lvm2Backend) GetSnapshotUsage(snapshotPath string) (float64, error) {
	ctx := context.Background()
	lvs, err := b.client.ListLogicalVolumes(ctx, &lvm2.ListLVOptions{
		Names: []string{snapshotPath},
	})
	if err != nil {
		return 0, err
	}
	if len(lvs) == 0 {
		return 0, fmt.Errorf("snapshot %s not found", snapshotPath)
	}
	usageStr := strings.TrimSpace(lvs[0].DataPercent)
	usage, err := strconv.ParseFloat(usageStr, 64)
	if err != nil {
		return 0, err
	}
	return usage, nil
}

func (b *lvm2Backend) GetVolumeGroupFreeSpace(vgName string) (uint64, error) {
	ctx := context.Background()
	vgs, err := b.client.ListVolumeGroups(ctx, &lvm2.ListVGOptions{
		Names: []string{vgName},
	})
	if err != nil {
		return 0, err
	}
	if len(vgs) == 0 {
		return 0, fmt.Errorf("volume group %s not found", vgName)
	}
	freeStr := strings.TrimSpace(vgs[0].Free)
	freeStr = strings.TrimSuffix(freeStr, "B")
	free, err := strconv.ParseUint(freeStr, 10, 64)
	if err != nil {
		return 0, err
	}
	return free, nil
}

func (b *lvm2Backend) ListVolumeGroups() ([]VolumeGroup, error) {
	ctx := context.Background()
	vgs, err := b.client.ListVolumeGroups(ctx, nil)
	if err != nil {
		return nil, err
	}
	res := make([]VolumeGroup, 0, len(vgs))
	for _, vg := range vgs {
		freeStr := strings.TrimSpace(vg.Free)
		freeStr = strings.TrimSuffix(freeStr, "B")
		free, err := strconv.ParseUint(freeStr, 10, 64)
		if err != nil {
			return nil, err
		}
		res = append(res, VolumeGroup{Name: vg.Name, Free: free})
	}
	return res, nil
}
