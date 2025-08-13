package device

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// RawDevice represents a generic block device opened from /dev.
type RawDevice struct {
	f         *os.File
	size      uint64
	blockSize uint64
}

// OpenRaw opens a block device at the given path and queries its size and block size.
func OpenRaw(path string) (*RawDevice, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeDevice == 0 || info.Mode()&os.ModeCharDevice != 0 {
		return nil, fmt.Errorf("%s is not a block device", path)
	}
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	size, err := unix.IoctlGetUint64(int(f.Fd()), unix.BLKGETSIZE64)
	if err != nil {
		f.Close()
		return nil, err
	}
	bs, err := unix.IoctlGetInt(int(f.Fd()), unix.BLKSSZGET)
	if err != nil {
		f.Close()
		return nil, err
	}
	return &RawDevice{f: f, size: size, blockSize: uint64(bs)}, nil
}

// Path returns the device path.
func (d *RawDevice) Path() string { return d.f.Name() }

// SizeBytes returns the total size of the device in bytes.
func (d *RawDevice) SizeBytes() uint64 { return d.size }

// BlockSize returns the logical block size of the device in bytes.
func (d *RawDevice) BlockSize() uint64 { return d.blockSize }

// Close closes the underlying file descriptor.
func (d *RawDevice) Close() error { return d.f.Close() }
