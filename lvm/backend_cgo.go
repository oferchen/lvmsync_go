//go:build linux && cgo

package lvm

import (
	"context"
	"fmt"

	"lvmsync_go/internal/sizeparse"
	"lvmsync_go/lvm/cgo"
)

type dmBackend struct{ dm cgo.LVM }

func newBackendWithCGO(dm cgo.LVM) lvmBackend { return &dmBackend{dm: dm} }

func newLVMBackend() lvmBackend { return newBackendWithCGO(cgo.New()) }

func (b *dmBackend) CreateSnapshot(ctx context.Context, lvPath, snapshotName, size string) error {
	bytes, percent, err := sizeparse.Parse(size)
	if err != nil {
		return err
	}
	if percent {
		return fmt.Errorf("percentage sizes not supported")
	}
	return b.dm.CreateSnapshot(lvPath, snapshotName, uint64(bytes))
}

func (b *dmBackend) RemoveSnapshot(ctx context.Context, snapshotPath string) error {
	return b.dm.RemoveLV(snapshotPath)
}

func (b *dmBackend) GetSnapshotUsage(ctx context.Context, snapshotPath string) (float64, error) {
	return b.dm.SnapshotUsage(snapshotPath)
}

func (b *dmBackend) GetVolumeGroupFreeSpace(ctx context.Context, vgName string) (uint64, error) {
	return b.dm.VGFree(vgName)
}

func (b *dmBackend) ListVolumeGroups(ctx context.Context, candidates []string) ([]VolumeGroup, error) {
	vgs, err := b.dm.ListVGs()
	if err != nil {
		return nil, err
	}
	include := make(map[string]bool)
	if len(candidates) > 0 {
		for _, c := range candidates {
			include[c] = true
		}
	}
	res := []VolumeGroup{}
	for _, vg := range vgs {
		if len(include) > 0 && !include[vg.Name] {
			continue
		}
		res = append(res, VolumeGroup{Name: vg.Name, Free: vg.Free})
	}
	return res, nil
}
