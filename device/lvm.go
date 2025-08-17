//go:build linux

package device

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"go.uber.org/zap"
	"golang.org/x/sys/unix"

	"lvmsync_go/internal/lock"
	"lvmsync_go/lvm"
)

// LVMDevice represents a logical volume managed by LVM.
type LVMDevice struct {
	f           *os.File
	path        string
	size        uint64
	blockSize   uint64
	cleanupPath string
	escalation  string
	logger      *zap.Logger
	lock        *lock.Lock
	runner      *Runner
}

// OpenLVM opens an LVM logical volume and queries its size and block size.
// Size information is obtained through the lvm package helpers.
func (r *Runner) OpenLVM(path string, cache *lvm.FDCache, escalation string, logger *zap.Logger) (*LVMDevice, error) {
	if r.openLVMOverride != nil {
		return r.openLVMOverride(path, cache, escalation, logger)
	}
	ctx := context.Background()
	exists, err := r.volumeExists(ctx, path)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, fmt.Errorf("volume %s does not exist", path)
	}
	auto, err := r.autoExtendEnabled(ctx, path)
	if err != nil {
		return nil, err
	}
	if auto {
		return nil, fmt.Errorf("auto-extend enabled for %s", path)
	}
	discard, err := r.discardEnabled(ctx, path)
	if err != nil {
		return nil, err
	}
	if !discard {
		return nil, fmt.Errorf("discard disabled for %s", path)
	}
	mounted, err := r.isMountedRW(path)
	if err != nil {
		return nil, err
	}
	if mounted {
		return nil, fmt.Errorf("device %s mounted", path)
	}
	vg, lv, err := lvm.ParseLVPath(path)
	if err != nil {
		return nil, err
	}
	lk, err := r.lockAcquire(vg, lv)
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		_ = lk.Release()
		return nil, err
	}
	bs, err := unix.IoctlGetInt(int(f.Fd()), unix.BLKSSZGET)
	if err != nil {
		f.Close()
		_ = lk.Release()
		return nil, err
	}
	size, err := lvm.GetVolumeSize(path, cache, logger)
	if err != nil {
		f.Close()
		_ = lk.Release()
		return nil, err
	}
	return &LVMDevice{f: f, path: path, size: size, blockSize: uint64(bs), escalation: escalation, logger: logger, lock: lk, runner: r}, nil
}

// Path returns the underlying device path.
func (d *LVMDevice) Path() string { return d.path }

// SizeBytes returns the logical volume size in bytes.
func (d *LVMDevice) SizeBytes() uint64 { return d.size }

// BlockSize returns the logical block size in bytes.
func (d *LVMDevice) BlockSize() uint64 { return d.blockSize }

// Close closes the device.
func (d *LVMDevice) Close() error {
	if d.f != nil {
		return d.f.Close()
	}
	return nil
}

var (
	generateSnapshot = func() string { return fmt.Sprintf("lvmsync_%d", time.Now().UnixNano()) }
	geteuid          = os.Geteuid
)

func (r *Runner) runLVM(ctx context.Context, escalation, cmdName string, args ...string) error {
	if err := lvm.VerifyEscalationCommand(escalation); err != nil {
		return err
	}
	if geteuid() != 0 {
		parts := strings.Fields(escalation)
		if len(parts) > 0 {
			lvmCmd := cmdName
			cmdName = parts[0]
			args = append(parts[1:], append([]string{lvmCmd}, args...)...)
		}
	}
	cmd := r.command.CommandContext(ctx, cmdName, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// Snapshot creates, activates, and opens an LVM snapshot of the device using
// the provided snapshotSize (e.g., "1G" or "20%").
func (d *LVMDevice) Snapshot(ctx context.Context, snapshotSize string) (Device, error) {
	vg, err := lvm.GetVolumeGroupName(d.path)
	if err != nil {
		return nil, err
	}
	snapName := generateSnapshot()
	if err := d.runner.runLVM(ctx, d.escalation, "lvcreate", "-s", "-n", snapName, "-L", snapshotSize, d.path); err != nil {
		return nil, err
	}
	snapPath := lvm.GetSnapshotDevicePath(snapName, vg, d.logger)
	if err := d.runner.runLVM(ctx, d.escalation, "lvchange", "-ay", snapPath); err != nil {
		_ = d.runner.runLVM(ctx, d.escalation, "lvremove", "-f", snapPath)
		return nil, err
	}
	cache := lvm.NewDeviceFDCache(d.logger)
	defer cache.Close()
	snapDev, err := d.runner.OpenLVM(snapPath, cache, d.escalation, d.logger)
	if err != nil {
		_ = d.runner.runLVM(ctx, d.escalation, "lvremove", "-f", snapPath)
		return nil, err
	}
	snapDev.cleanupPath = snapPath
	return snapDev, nil
}

// Cleanup removes the snapshot if one was created.
func (d *LVMDevice) Cleanup(ctx context.Context) error {
	if d.lock != nil {
		_ = d.lock.Release()
		d.lock = nil
	}
	if d.cleanupPath == "" {
		return nil
	}
	if err := d.runner.runLVM(ctx, d.escalation, "lvremove", "-f", d.cleanupPath); err != nil {
		return err
	}
	d.cleanupPath = ""
	return nil
}
