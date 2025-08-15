//go:build linux

package device

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"unsafe"

	"go.uber.org/zap"
	"golang.org/x/sys/unix"
)

// RawDevice represents a generic block device opened from /dev.
type RawDevice struct {
	f            *os.File
	size         uint64
	blockSize    uint64
	freezeIssued bool
	logger       *zap.Logger
}

// OpenRaw opens a block device at the given path and queries its size and block
// size. If offline is false, fsFreezeCmdPath and fsThawCmdPath must be commands
// that successfully freeze and thaw the filesystem around the device access.
func OpenRaw(
	ctx context.Context,
	path string,
	offline bool,
	fsFreezeCmdPath string,
	fsFreezeCmdArgs []string,
	fsThawCmdPath string,
	fsThawCmdArgs []string,
	logger *zap.Logger,
) (_ *RawDevice, err error) {
	d := &RawDevice{logger: logger}
	if !offline {
		if fsFreezeCmdPath == "" || fsThawCmdPath == "" {
			return nil, fmt.Errorf("raw sources require --offline or --fs-freeze-command/--fs-thaw-command")
		}
		if err := validateCmd(fsFreezeCmdPath, fsFreezeCmdArgs); err != nil {
			return nil, fmt.Errorf("invalid freeze command: %w", err)
		}
		if err := validateCmd(fsThawCmdPath, fsThawCmdArgs); err != nil {
			return nil, fmt.Errorf("invalid thaw command: %w", err)
		}
		if d.logger != nil {
			d.logger.Info("fs freeze start", zap.String("command", fsFreezeCmdPath), zap.Strings("args", fsFreezeCmdArgs))
		}
		if err = exec.CommandContext(ctx, fsFreezeCmdPath, fsFreezeCmdArgs...).Run(); err != nil {
			return nil, fmt.Errorf("freeze command failed: %w", err)
		}
		if d.logger != nil {
			d.logger.Info("fs freeze complete")
		}
		d.freezeIssued = true
		defer func() {
			if err != nil {
				_ = d.Cleanup(ctx, fsThawCmdPath, fsThawCmdArgs)
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
	if d.logger != nil {
		d.logger.Info("raw device info",
			zap.String("path", path),
			zap.Uint64("size_bytes", size),
			zap.Uint64("block_size_bytes", uint64(bs)))
	}
	return d, nil
}

// Path returns the device path.
func (d *RawDevice) Path() string { return d.f.Name() }

// SizeBytes returns the total size of the device in bytes.
func (d *RawDevice) SizeBytes() uint64 { return d.size }

// BlockSize returns the logical block size of the device in bytes.
func (d *RawDevice) BlockSize() uint64 { return d.blockSize }

// Close closes the underlying file descriptor.
func (d *RawDevice) Close() error {
	err := d.f.Close()
	if d.logger != nil {
		if err != nil {
			d.logger.Error("raw device close failed", zap.String("path", d.Path()), zap.Error(err))
		} else {
			d.logger.Info("raw device closed", zap.String("path", d.Path()))
		}
	}
	return err
}

// Snapshot returns the device itself for raw block devices.
func (d *RawDevice) Snapshot(context.Context, string) (Device, error) { return d, nil }

// Cleanup thaws the filesystem if a freeze command was issued.
func (d *RawDevice) Cleanup(ctx context.Context, fsThawCmdPath string, fsThawCmdArgs []string) error {
	if d.freezeIssued {
		if err := validateCmd(fsThawCmdPath, fsThawCmdArgs); err != nil {
			if d.logger != nil {
				d.logger.Error("fs thaw failed", zap.Error(err))
			}
			return fmt.Errorf("invalid thaw command: %w", err)
		}
		if d.logger != nil {
			d.logger.Info("fs thaw start", zap.String("command", fsThawCmdPath), zap.Strings("args", fsThawCmdArgs))
		}
		if err := exec.CommandContext(ctx, fsThawCmdPath, fsThawCmdArgs...).Run(); err != nil {
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

// validateCmd ensures the command path and arguments are suitable for execution.
func validateCmd(path string, args []string) error {
	if path == "" {
		return fmt.Errorf("command path is empty")
	}
	if strings.ContainsRune(path, '\x00') {
		return fmt.Errorf("command path contains NUL byte")
	}
	for _, a := range args {
		if strings.ContainsRune(a, '\x00') {
			return fmt.Errorf("command argument contains NUL byte")
		}
	}
	if _, err := exec.LookPath(path); err != nil {
		return fmt.Errorf("%s: %w", path, err)
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
