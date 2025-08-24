package device

import (
	"context"
	"fmt"
	"os"

	"go.uber.org/zap"
	"golang.org/x/sys/unix"
)

// FileDevice represents a regular file used as a block device.
type FileDevice struct {
	f         *os.File
	size      uint64
	blockSize uint64
	logger    *zap.Logger
}

// OpenFile opens a regular file and reports its size and filesystem block size.
// When readonly is true the file is opened with os.O_RDONLY. logger must be
// non-nil.
func OpenFile(path string, readonly bool, logger *zap.Logger) (*FileDevice, error) {
	info, err := os.Stat(path)
	if err != nil {
		logger.Error("file_device_open_failed", zap.String("path", path), zap.Error(err))
		return nil, err
	}
	if !info.Mode().IsRegular() {
		err := fmt.Errorf("%s is not a regular file", path)
		logger.Error("file_device_open_failed", zap.String("path", path), zap.Error(err))
		return nil, err
	}
	var f *os.File
	if readonly {
		f, err = os.Open(path)
	} else {
		f, err = os.OpenFile(path, os.O_RDWR, 0)
	}
	if err != nil {
		logger.Error("file_device_open_failed", zap.String("path", path), zap.Error(err))
		return nil, err
	}
	logger.Info("file_device_opened", zap.String("path", path))
	var st unix.Stat_t
	if err := unix.Fstat(int(f.Fd()), &st); err != nil {
		f.Close()
		logger.Error("file_device_stat_failed", zap.String("path", path), zap.Error(err))
		return nil, err
	}
	size := uint64(info.Size())
	block := uint64(st.Blksize)
	logger.Info("file_device_info", zap.String("path", path), zap.Uint64("size_bytes", size), zap.Uint64("block_size_bytes", block))
	return &FileDevice{
		f:         f,
		size:      size,
		blockSize: block,
		logger:    logger,
	}, nil
}

// Path returns the file path.
func (d *FileDevice) Path() string { return d.f.Name() }

// SizeBytes returns the file size.
func (d *FileDevice) SizeBytes() uint64 { return d.size }

// BlockSize returns the filesystem block size.
func (d *FileDevice) BlockSize() uint64 { return d.blockSize }

// Identity returns size information for the file.
func (d *FileDevice) Identity(context.Context) (DeviceIdentity, error) {
	return DeviceIdentity{SizeBytes: d.SizeBytes()}, nil
}

// AppendWAL is a no-op for file devices.
func (d *FileDevice) AppendWAL(r Range) error { return nil }

// RecoverWAL is a no-op for file devices.
func (d *FileDevice) RecoverWAL(fn func(Range) error) error { return nil }

// Close closes the underlying file descriptor.
func (d *FileDevice) Close() error {
	err := d.f.Close()
	if err != nil {
		d.logger.Error("file_device_close_failed", zap.String("path", d.Path()), zap.Error(err))
	} else {
		d.logger.Info("file_device_closed", zap.String("path", d.Path()))
	}
	return err
}

// Snapshot returns the device itself for regular files. It verifies the
// underlying file descriptor is still open to surface errors when called on a
// closed device.
func (d *FileDevice) Snapshot(context.Context, string) (Device, error) {
	if _, err := d.f.Stat(); err != nil {
		return nil, err
	}
	return d, nil
}

// Cleanup is a no-op for regular files.
func (d *FileDevice) Cleanup(context.Context) error {
	d.logger.Info("file_device_cleanup", zap.String("path", d.Path()))
	return nil
}
