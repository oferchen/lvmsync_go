package lvm

import (
	"context"
	"fmt"

	"github.com/oferchen/lvmsync_go/internal/sizeparse"
	"github.com/oferchen/lvmsync_go/lvm/cgo"
)

// cgoBackend implements lvmBackend using a cgo.LVM implementation.
type cgoBackend struct{ lvm cgo.LVM }

func newBackendWithCGO(l cgo.LVM) lvmBackend { return &cgoBackend{lvm: l} }

// newLVMBackend constructs the default backend.
func newLVMBackend() lvmBackend { return newBackendWithCGO(cgo.New()) }

func (b *cgoBackend) CreateSnapshot(_ context.Context, lvPath, snapshotName, size string) error {
	bytes, percent, err := sizeparse.Parse(size)
	if err != nil {
		return fmt.Errorf("parse size %q: %w", size, err)
	}
	if percent {
		return fmt.Errorf("percentage sizes not supported")
	}
	if err := b.lvm.CreateSnapshot(lvPath, snapshotName, bytes); err != nil {
		return fmt.Errorf("cgo create snapshot: %w", err)
	}
	return nil
}

func (b *cgoBackend) RemoveSnapshot(_ context.Context, snapshotPath string) error {
	if err := b.lvm.RemoveLV(snapshotPath); err != nil {
		return fmt.Errorf("cgo remove snapshot: %w", err)
	}
	return nil
}

func (b *cgoBackend) GetSnapshotUsage(_ context.Context, snapshotPath string) (float64, error) {
	usage, err := b.lvm.SnapshotUsage(snapshotPath)
	if err != nil {
		return 0, fmt.Errorf("cgo snapshot usage: %w", err)
	}
	return usage, nil
}

func (b *cgoBackend) GetVolumeGroupFreeSpace(_ context.Context, vgName string) (uint64, error) {
	free, err := b.lvm.VGFree(vgName)
	if err != nil {
		return 0, fmt.Errorf("cgo vg free space: %w", err)
	}
	return free, nil
}

func (b *cgoBackend) ListVolumeGroups(_ context.Context, candidates []string) ([]VolumeGroup, error) {
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

func (b *cgoBackend) CreateLogicalVolume(_ context.Context, vgName, lvName string, sizeBytes uint64) error {
	if err := b.lvm.CreateLV(vgName, lvName, sizeBytes); err != nil {
		return fmt.Errorf("cgo create lv: %w", err)
	}
	return nil
}
