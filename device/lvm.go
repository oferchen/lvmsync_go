package device

import (
	"os"

	"go.uber.org/zap"
	"golang.org/x/sys/unix"

	"lvmsync_go/lvm"
)

// LVMDevice represents a logical volume managed by LVM.
type LVMDevice struct {
	f         *os.File
	size      uint64
	blockSize uint64
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
	return &LVMDevice{f: f, size: size, blockSize: uint64(bs)}, nil
}

// Path returns the underlying device path.
func (d *LVMDevice) Path() string { return d.f.Name() }

// SizeBytes returns the logical volume size in bytes.
func (d *LVMDevice) SizeBytes() uint64 { return d.size }

// BlockSize returns the logical block size in bytes.
func (d *LVMDevice) BlockSize() uint64 { return d.blockSize }

// Close closes the device.
func (d *LVMDevice) Close() error { return d.f.Close() }
