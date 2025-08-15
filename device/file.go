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
func OpenFile(path string, logger *zap.Logger) (*FileDevice, error) {
	info, err := os.Stat(path)
	if err != nil {
		if logger != nil {
			logger.Error("file device open failed", zap.String("path", path), zap.Error(err))
		}
		return nil, err
	}
	if !info.Mode().IsRegular() {
		err := fmt.Errorf("%s is not a regular file", path)
		if logger != nil {
			logger.Error("file device open failed", zap.String("path", path), zap.Error(err))
		}
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		if logger != nil {
			logger.Error("file device open failed", zap.String("path", path), zap.Error(err))
		}
		return nil, err
	}
	if logger != nil {
		logger.Info("file device opened", zap.String("path", path))
	}
	var st unix.Stat_t
	if err := unix.Fstat(int(f.Fd()), &st); err != nil {
		f.Close()
		if logger != nil {
			logger.Error("file device stat failed", zap.String("path", path), zap.Error(err))
		}
		return nil, err
	}
	size := uint64(info.Size())
	block := uint64(st.Blksize)
	if logger != nil {
		logger.Info("file device info", zap.String("path", path), zap.Uint64("size_bytes", size), zap.Uint64("block_size", block))
	}
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

// Close closes the underlying file descriptor.
func (d *FileDevice) Close() error {
	err := d.f.Close()
	if d.logger != nil {
		if err != nil {
			d.logger.Error("file device close failed", zap.String("path", d.Path()), zap.Error(err))
		} else {
			d.logger.Info("file device closed", zap.String("path", d.Path()))
		}
	}
	return err
}

// Snapshot returns the device itself for regular files.
func (d *FileDevice) Snapshot(context.Context, string) (Device, error) {
	if d.logger != nil {
		d.logger.Info("file device snapshot", zap.String("path", d.Path()))
	}
	return d, nil
}

// Cleanup is a no-op for regular files.
func (d *FileDevice) Cleanup(context.Context, string, []string) error {
	if d.logger != nil {
		d.logger.Info("file device cleanup", zap.String("path", d.Path()))
	}
	return nil
}
