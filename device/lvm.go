package device

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"go.uber.org/zap"
	"golang.org/x/sys/unix"

	"lvmsync_go/lvm"
)

// LVMDevice represents a logical volume managed by LVM.
type LVMDevice struct {
	f           *os.File
	path        string
	size        uint64
	blockSize   uint64
	cleanupPath string
}

// OpenLVM opens an LVM logical volume and queries its size and block size.
// Size information is obtained through the lvm package helpers.
func OpenLVM(path string) (*LVMDevice, error) {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	bs, err := unix.IoctlGetInt(int(f.Fd()), unix.BLKSSZGET)
	if err != nil {
		f.Close()
		return nil, err
	}
	size, err := lvm.GetVolumeSize(path, zap.NewNop())
	if err != nil {
		f.Close()
		return nil, err
	}
	return &LVMDevice{f: f, path: path, size: size, blockSize: uint64(bs)}, nil
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
	execCommand      = exec.CommandContext
	generateSnapshot = func() string { return fmt.Sprintf("lvmsync_%d", time.Now().UnixNano()) }
	geteuid          = os.Geteuid
	openLVMFunc      = OpenLVM
)

func runLVM(ctx context.Context, name string, args ...string) error {
	if geteuid() != 0 {
		args = append([]string{"-n", name}, args...)
		name = "sudo"
	}
	cmd := execCommand(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// Snapshot creates, activates, and opens an LVM snapshot of the device.
func (d *LVMDevice) Snapshot(ctx context.Context) (Device, error) {
	vg, err := lvm.GetVolumeGroupName(d.path)
	if err != nil {
		return nil, err
	}
	snapName := generateSnapshot()
	if err := runLVM(ctx, "lvcreate", "-s", "-n", snapName, "-L", "1G", d.path); err != nil {
		return nil, err
	}
	snapPath := lvm.GetSnapshotDevicePath(snapName, vg, zap.NewNop())
	if err := runLVM(ctx, "lvchange", "-ay", snapPath); err != nil {
		_ = runLVM(ctx, "lvremove", "-f", snapPath)
		return nil, err
	}
	snapDev, err := openLVMFunc(snapPath)
	if err != nil {
		_ = runLVM(ctx, "lvremove", "-f", snapPath)
		return nil, err
	}
	snapDev.cleanupPath = snapPath
	return snapDev, nil
}

// Cleanup removes the snapshot if one was created.
func (d *LVMDevice) Cleanup(ctx context.Context) error {
	if d.cleanupPath == "" {
		return nil
	}
	if err := runLVM(ctx, "lvremove", "-f", d.cleanupPath); err != nil {
		return err
	}
	d.cleanupPath = ""
	return nil
}
