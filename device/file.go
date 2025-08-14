package device

import (
	"context"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// FileDevice represents a regular file used as a block device.
type FileDevice struct {
	f         *os.File
	size      uint64
	blockSize uint64
}

// OpenFile opens a regular file and reports its size and filesystem block size.
func OpenFile(path string) (*FileDevice, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", path)
	}
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	var st unix.Stat_t
	if err := unix.Fstat(int(f.Fd()), &st); err != nil {
		f.Close()
		return nil, err
	}
	return &FileDevice{
		f:         f,
		size:      uint64(info.Size()),
		blockSize: uint64(st.Blksize),
	}, nil
}

// Path returns the file path.
func (d *FileDevice) Path() string { return d.f.Name() }

// SizeBytes returns the file size.
func (d *FileDevice) SizeBytes() uint64 { return d.size }

// BlockSize returns the filesystem block size.
func (d *FileDevice) BlockSize() uint64 { return d.blockSize }

// Close closes the underlying file descriptor.
func (d *FileDevice) Close() error { return d.f.Close() }

// Snapshot returns the device itself for regular files.
func (d *FileDevice) Snapshot(context.Context, string) (Device, error) { return d, nil }

// Cleanup is a no-op for regular files.
func (d *FileDevice) Cleanup(context.Context, string, []string) error { return nil }
