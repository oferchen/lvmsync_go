package device

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"unsafe"

	"go.uber.org/zap"
	"golang.org/x/sys/unix"
)

// RawDevice represents a generic block device opened from /dev.
type RawDevice struct {
	f            *os.File
	size         uint64
	blockSize    uint64
	fsFreezeCmd  string
	fsThawCmd    string
	freezeIssued bool
	logger       *zap.Logger
}

// OpenRaw opens a block device at the given path and queries its size and block size.
// If offline is false, fsFreezeCmd and fsThawCmd must be commands that successfully
// freeze and thaw the filesystem around the device access.
func OpenRaw(ctx context.Context, path string, offline bool, fsFreezeCmd, fsThawCmd string, logger *zap.Logger) (_ *RawDevice, err error) {
	d := &RawDevice{fsFreezeCmd: fsFreezeCmd, fsThawCmd: fsThawCmd, logger: logger}
	if !offline {
		if fsFreezeCmd == "" || fsThawCmd == "" {
			return nil, fmt.Errorf("raw sources require --offline or --fs-freeze-command/--fs-thaw-command")
		}
		if d.logger != nil {
			d.logger.Info("fs freeze start", zap.String("command", fsFreezeCmd))
		}
		if err = exec.CommandContext(ctx, "sh", "-c", fsFreezeCmd).Run(); err != nil {
			return nil, fmt.Errorf("freeze command failed: %w", err)
		}
		if d.logger != nil {
			d.logger.Info("fs freeze complete")
		}
		d.freezeIssued = true
		defer func() {
			if err != nil {
				_ = d.Cleanup(ctx)
			}
		}()
	}
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
	size, err := ioctlGetUint64(int(f.Fd()), unix.BLKGETSIZE64)
	if err != nil {
		f.Close()
		return nil, err
	}
	bs, err := unix.IoctlGetInt(int(f.Fd()), unix.BLKSSZGET)
	if err != nil {
		f.Close()
		return nil, err
	}
	d.f = f
	d.size = size
	d.blockSize = uint64(bs)
	return d, nil
}

// Path returns the device path.
func (d *RawDevice) Path() string { return d.f.Name() }

// SizeBytes returns the total size of the device in bytes.
func (d *RawDevice) SizeBytes() uint64 { return d.size }

// BlockSize returns the logical block size of the device in bytes.
func (d *RawDevice) BlockSize() uint64 { return d.blockSize }

// Close closes the underlying file descriptor.
func (d *RawDevice) Close() error { return d.f.Close() }

// Snapshot returns the device itself for raw block devices.
func (d *RawDevice) Snapshot(context.Context, string) (Device, error) { return d, nil }

// Cleanup thaws the filesystem if a freeze command was issued.
func (d *RawDevice) Cleanup(ctx context.Context) error {
	if d.freezeIssued && d.fsThawCmd != "" {
		if d.logger != nil {
			d.logger.Info("fs thaw start", zap.String("command", d.fsThawCmd))
		}
		if err := exec.CommandContext(ctx, "sh", "-c", d.fsThawCmd).Run(); err != nil {
			if d.logger != nil {
				d.logger.Error("fs thaw failed", zap.Error(err))
			}
			return fmt.Errorf("thaw command failed: %w", err)
		}
		if d.logger != nil {
			d.logger.Info("fs thaw complete")
		}
	}
	return nil
}

// ioctlGetUint64 performs an ioctl call expecting a 64-bit unsigned result.
func ioctlGetUint64(fd int, req uint) (uint64, error) {
	var v uint64
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), uintptr(req), uintptr(unsafe.Pointer(&v)))
	if errno != 0 {
		return 0, errno
	}
	return v, nil
}
