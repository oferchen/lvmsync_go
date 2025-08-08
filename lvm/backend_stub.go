//go:build !linux || !cgo

package lvm

import (
	"context"
	"fmt"

	"lvmsync_go/internal/sizeparse"
	"lvmsync_go/lvm/cgo"
)

type cgoBackend struct{ lvm cgo.LVM }

func newBackendWithCGO(lvm cgo.LVM) lvmBackend { return &cgoBackend{lvm: lvm} }

func newLVMBackend() lvmBackend { return newBackendWithCGO(cgo.New()) }

func (b *cgoBackend) CreateSnapshot(ctx context.Context, lvPath, snapshotName, size string) error {
	bytes, percent, err := sizeparse.Parse(size)
	if err != nil {
		return fmt.Errorf("parse size %q: %w", size, err)
	}
	if percent {
		return fmt.Errorf("percentage sizes not supported")
	}

	u := uint64(bytes)
	if bytes < 0 || float64(u) != bytes {
		return fmt.Errorf("size %q overflows uint64", size)
	}

	if err := b.lvm.CreateSnapshot(lvPath, snapshotName, u); err != nil {
		return fmt.Errorf("cgo create snapshot: %w", err)
	}
	return nil
}

func (b *cgoBackend) RemoveSnapshot(ctx context.Context, snapshotPath string) error {
	if err := b.lvm.RemoveLV(snapshotPath); err != nil {
		return fmt.Errorf("cgo remove snapshot: %w", err)
	}
	return nil
}

func (b *cgoBackend) GetSnapshotUsage(ctx context.Context, snapshotPath string) (float64, error) {
	usage, err := b.lvm.SnapshotUsage(snapshotPath)
	if err != nil {
		return 0, fmt.Errorf("cgo snapshot usage: %w", err)
	}
	return usage, nil
}

func (b *cgoBackend) GetVolumeGroupFreeSpace(ctx context.Context, vgName string) (uint64, error) {
	free, err := b.lvm.VGFree(vgName)
	if err != nil {
		return 0, fmt.Errorf("cgo vg free space: %w", err)
	}
	return free, nil
}

func (b *cgoBackend) ListVolumeGroups(ctx context.Context, candidates []string) ([]VolumeGroup, error) {
	vgs, err := b.lvm.ListVGs()
	if err != nil {
		return nil, fmt.Errorf("cgo list volume groups: %w", err)
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
