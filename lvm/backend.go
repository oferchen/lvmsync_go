package lvm

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	golvm "github.com/nak3/go-lvm"
	"lvmsync_go/internal/sizeparse"
)

type lvmBackend interface {
	CreateSnapshot(ctx context.Context, lvPath, snapshotName, size string) error
	RemoveSnapshot(ctx context.Context, snapshotPath string) error
	GetSnapshotUsage(ctx context.Context, snapshotPath string) (float64, error)
	GetVolumeGroupFreeSpace(ctx context.Context, vgName string) (uint64, error)
	ListVolumeGroups(ctx context.Context, candidates []string) ([]VolumeGroup, error)
}

type golvmBackend struct{}

func newLVMBackend() lvmBackend { return &golvmBackend{} }

func (b *golvmBackend) openVG(name, mode string) (*golvm.VgObject, error) {
	vgo := &golvm.VgObject{}
	vgo.Vgt = golvm.VgOpen(name, mode)
	if vgo.Vgt == nil {
		return nil, fmt.Errorf("failed to open volume group %s", name)
	}
	return vgo, nil
}

func (b *golvmBackend) CreateSnapshot(ctx context.Context, lvPath, snapshotName, size string) error {
	trimmed := strings.TrimPrefix(lvPath, "/dev/")
	parts := strings.Split(trimmed, "/")
	if len(parts) != 2 {
		return fmt.Errorf("invalid LV path: %s", lvPath)
	}
	vgName, lvName := parts[0], parts[1]

	bytes, percent, err := sizeparse.Parse(size)
	if err != nil {
		return err
	}
	if percent {
		return fmt.Errorf("percentage sizes not supported")
	}

	vg, err := b.openVG(vgName, "w")
	if err != nil {
		return err
	}
	defer vg.Close()

	lv, err := vg.LvFromName(lvName)
	if err != nil {
		return err
	}
	_, err = lv.Snapshot(snapshotName, uint64(bytes))
	return err
}

func (b *golvmBackend) RemoveSnapshot(ctx context.Context, snapshotPath string) error {
	trimmed := strings.TrimPrefix(snapshotPath, "/dev/")
	parts := strings.Split(trimmed, "/")
	if len(parts) != 2 {
		return fmt.Errorf("invalid snapshot path: %s", snapshotPath)
	}
	vgName, lvName := parts[0], parts[1]

	vg, err := b.openVG(vgName, "w")
	if err != nil {
		return err
	}
	defer vg.Close()

	lv, err := vg.LvFromName(lvName)
	if err != nil {
		return err
	}
	return lv.Remove()
}

func (b *golvmBackend) GetSnapshotUsage(ctx context.Context, snapshotPath string) (float64, error) {
	trimmed := strings.TrimPrefix(snapshotPath, "/dev/")
	parts := strings.Split(trimmed, "/")
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid snapshot path: %s", snapshotPath)
	}
	vgName, lvName := parts[0], parts[1]

	vg, err := b.openVG(vgName, "r")
	if err != nil {
		return 0, err
	}
	defer vg.Close()

	lv, err := vg.LvFromName(lvName)
	if err != nil {
		return 0, err
	}
	prop, err := lv.GetProperty("data_percent")
	if err != nil {
		return 0, err
	}
	usage, err := strconv.ParseFloat(strings.TrimSpace(prop.Str), 64)
	if err != nil {
		return 0, err
	}
	return usage, nil
}

func (b *golvmBackend) GetVolumeGroupFreeSpace(ctx context.Context, vgName string) (uint64, error) {
	vg, err := b.openVG(vgName, "r")
	if err != nil {
		return 0, err
	}
	defer vg.Close()
	return vg.GetFreeSize(), nil
}

func (b *golvmBackend) ListVolumeGroups(ctx context.Context, candidates []string) ([]VolumeGroup, error) {
	names := golvm.ListVgNames()
	include := make(map[string]bool)
	if len(candidates) > 0 {
		for _, c := range candidates {
			include[c] = true
		}
	}
	res := []VolumeGroup{}
	for _, name := range names {
		if len(include) > 0 && !include[name] {
			continue
		}
		vg, err := b.openVG(name, "r")
		if err != nil {
			return nil, err
		}
		free := vg.GetFreeSize()
		vg.Close()
		res = append(res, VolumeGroup{Name: name, Free: free})
	}
	return res, nil
}
