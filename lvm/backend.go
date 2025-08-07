package lvm

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	golvm "github.com/nak3/go-lvm"
	"go.uber.org/zap"
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

func parseLVPath(path string) (string, string, error) {
	trimmed := strings.TrimPrefix(path, "/dev/")
	parts := strings.Split(trimmed, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid logical volume path: %s", path)
	}
	return parts[0], parts[1], nil
}

func (b *golvmBackend) withLV(path, mode string, fn func(*golvm.LvObject) error) error {
	vgName, lvName, err := parseLVPath(path)
	if err != nil {
		return err
	}
	vg, err := b.openVG(vgName, mode)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := vg.Close(); closeErr != nil {
			zap.L().Warn("failed to close volume group", zap.Error(closeErr))
		}
	}()
	lv, err := vg.LvFromName(lvName)
	if err != nil {
		return err
	}
	return fn(lv)
}

func (b *golvmBackend) CreateSnapshot(ctx context.Context, lvPath, snapshotName, size string) error {
	bytes, percent, err := sizeparse.Parse(size)
	if err != nil {
		return err
	}
	if percent {
		return fmt.Errorf("percentage sizes not supported")
	}

	return b.withLV(lvPath, "w", func(lv *golvm.LvObject) error {
		_, err := lv.Snapshot(snapshotName, uint64(bytes))
		return err
	})
}

func (b *golvmBackend) RemoveSnapshot(ctx context.Context, snapshotPath string) error {
	return b.withLV(snapshotPath, "w", func(lv *golvm.LvObject) error {
		return lv.Remove()
	})
}

func (b *golvmBackend) GetSnapshotUsage(ctx context.Context, snapshotPath string) (float64, error) {
	var usage float64
	err := b.withLV(snapshotPath, "r", func(lv *golvm.LvObject) error {
		prop, err := lv.GetProperty("data_percent")
		if err != nil {
			return err
		}
		usage, err = strconv.ParseFloat(strings.TrimSpace(prop.Str), 64)
		return err
	})
	return usage, err
}

func (b *golvmBackend) GetVolumeGroupFreeSpace(ctx context.Context, vgName string) (uint64, error) {
	vg, err := b.openVG(vgName, "r")
	if err != nil {
		return 0, err
	}
	defer func() {
		if err := vg.Close(); err != nil {
			zap.L().Warn("failed to close volume group", zap.Error(err))
		}
	}()
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
		if err := vg.Close(); err != nil {
			zap.L().Error("failed to close volume group", zap.String("name", name), zap.Error(err))
		}
		res = append(res, VolumeGroup{Name: name, Free: free})
	}
	return res, nil
}
