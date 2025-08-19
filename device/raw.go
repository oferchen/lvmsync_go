//go:build linux

package device

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"go.uber.org/zap"
	"golang.org/x/sys/unix"
	"golang.org/x/term"

	"lvmsync_go/internal/privilege"
	"lvmsync_go/remote"
)

// RawDevice represents a generic block device opened from /dev.
type RawDevice struct {
	f             *os.File
	size          uint64
	blockSize     uint64
	freezeIssued  bool
	logger        *zap.Logger
	freezeTimeout time.Duration
	thawTimeout   time.Duration
	thawCmdPath   string
	thawCmdArgs   []string
	runner        *Runner
	wal           *WAL
}

// prepareFreeze validates and runs the filesystem freeze command when offline is false.
func prepareFreeze(
	ctx context.Context,
	offline bool,
	fsFreezeCmdPath string,
	fsFreezeCmdArgs []string,
	fsThawCmdPath string,
	fsThawCmdArgs []string,
	freezeTimeout time.Duration,
	logger *zap.Logger,
	runner *Runner,
) (bool, error) {
	if offline || fsFreezeCmdPath != "" || fsThawCmdPath != "" {
		if err := confirmOverwrite(ctx, os.Stdin, os.Stderr, term.IsTerminal); err != nil {
			return false, err
		}
	}
	if offline {
		return false, nil
	}
	if fsFreezeCmdPath == "" || fsThawCmdPath == "" {
		return false, fmt.Errorf("raw sources require --offline or --fs-freeze-command/--fs-thaw-command")
	}
	if err := validateCmd(fsFreezeCmdPath, fsFreezeCmdArgs); err != nil {
		return false, fmt.Errorf("invalid freeze command: %w", err)
	}
	if err := validateCmd(fsThawCmdPath, fsThawCmdArgs); err != nil {
		return false, fmt.Errorf("invalid thaw command: %w", err)
	}
	logger.Info("fs_freeze_start", zap.String("command", fsFreezeCmdPath), zap.Strings("args", fsFreezeCmdArgs))
	freezeCtx := ctx
	if _, ok := ctx.Deadline(); !ok && freezeTimeout > 0 {
		var cancel context.CancelFunc
		freezeCtx, cancel = context.WithTimeout(ctx, freezeTimeout)
		defer cancel()
	}
	out, cmdErr := runner.command.CommandContext(freezeCtx, fsFreezeCmdPath, fsFreezeCmdArgs...).CombinedOutput()
	if cmdErr != nil {
		output := strings.TrimSpace(string(out))
		logger.Error("fs_freeze_failed", zap.Error(cmdErr), zap.String("output", output))
		return false, fmt.Errorf("freeze command failed: %w: %s", cmdErr, output)
	}
	logger.Info("fs_freeze_complete")
	return true, nil
}

// openDevice ensures the path is a block device and opens it for reading and writing.
// Callers must ensure the necessary privilege before invoking this function.
func openDevice(path string) (*os.File, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeDevice == 0 || info.Mode()&os.ModeCharDevice != 0 {
		return nil, fmt.Errorf("%s is not a block device", path)
	}
	return os.OpenFile(path, os.O_RDWR, 0)
}

// queryDeviceInfo retrieves the size and block size for an opened block device and logs them.
func queryDeviceInfo(f *os.File, path string, logger *zap.Logger) (uint64, uint64, error) {
	size, err := ioctlGetUint64(int(f.Fd()), unix.BLKGETSIZE64)
	if err != nil {
		return 0, 0, err
	}
	bs, err := unix.IoctlGetInt(int(f.Fd()), unix.BLKSSZGET)
	if err != nil {
		return 0, 0, err
	}
	logger.Info("raw_device_info",
		zap.String("path", path),
		zap.Uint64("size_bytes", size),
		zap.Uint64("block_size_bytes", uint64(bs)))
	return size, uint64(bs), nil
}

// OpenRaw opens a block device at the given path and queries its size and block
// size. If offline is false, fsFreezeCmdPath and fsThawCmdPath must be commands
// that successfully freeze and thaw the filesystem around the device access. logger must be non-nil.
func OpenRaw(
	ctx context.Context,
	path string,
	offline bool,
	fsFreezeCmdPath string,
	fsFreezeCmdArgs []string,
	fsThawCmdPath string,
	fsThawCmdArgs []string,
	freezeTimeout time.Duration,
	thawTimeout time.Duration,
	esc privilege.Escalator,
	logger *zap.Logger,
	runner *Runner,
) (_ *RawDevice, err error) {
	if esc == nil {
		esc = privilege.New(ctx)
	}
	if err := esc.Ensure(ctx); err != nil {
		return nil, err
	}
	if runner == nil {
		runner = NewRunner()
	}
	d := &RawDevice{
		logger: logger, freezeTimeout: freezeTimeout, thawTimeout: thawTimeout,
		thawCmdPath: fsThawCmdPath, thawCmdArgs: fsThawCmdArgs, runner: runner,
	}
	freezeIssued, err := prepareFreeze(ctx, offline, fsFreezeCmdPath, fsFreezeCmdArgs, fsThawCmdPath, fsThawCmdArgs, freezeTimeout, logger, runner)
	if err != nil {
		return nil, err
	}
	d.freezeIssued = freezeIssued
	if freezeIssued {
		defer func() {
			if err != nil {
				_ = d.Cleanup(ctx)
			}
		}()
	}
	f, err := openDevice(path)
	if err != nil {
		return nil, err
	}
	size, blockSize, err := queryDeviceInfo(f, path, logger)
	if err != nil {
		f.Close()
		return nil, err
	}
	d.f = f
	d.size = size
	d.blockSize = blockSize
	return d, nil
}

// Path returns the device path.
func (d *RawDevice) Path() string { return d.f.Name() }

// SizeBytes returns the total size of the device in bytes.
func (d *RawDevice) SizeBytes() uint64 { return d.size }

// BlockSize returns the logical block size of the device in bytes.
func (d *RawDevice) BlockSize() uint64 { return d.blockSize }

// Identity gathers size, kernel uuid and GPT information for the device.
func (d *RawDevice) Identity(ctx context.Context) (DeviceIdentity, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, identityTimeout)
		defer cancel()
	}
	if blkidPath == "" {
		return DeviceIdentity{}, fmt.Errorf("blkid not found")
	}
	var st unix.Stat_t
	if err := unix.Stat(d.Path(), &st); err != nil {
		return DeviceIdentity{}, err
	}
	id := DeviceIdentity{
		SizeBytes: d.SizeBytes(),
		Major:     uint32(unix.Major(uint64(st.Rdev))),
		Minor:     uint32(unix.Minor(uint64(st.Rdev))),
	}
	out, err := exec.CommandContext(ctx, blkidPath, "-o", "value", "-s", "UUID", d.Path()).Output()
	if err != nil {
		return DeviceIdentity{}, err
	}
	uuid := strings.TrimSpace(string(out))
	id.KernelUUID = uuid
	id.FSUUID = uuid
	if out, err = exec.CommandContext(ctx, blkidPath, "-o", "value", "-s", "PTUUID", d.Path()).Output(); err == nil {
		id.GPTUUID = strings.TrimSpace(string(out))
	}
	return id, nil
}

// SetWAL attaches a WAL to the device.
func (d *RawDevice) SetWAL(w *WAL) { d.wal = w }

// AppendWAL records the applied range in the attached WAL.
func (d *RawDevice) AppendWAL(r Range) error {
	if d.wal == nil {
		return nil
	}
	return d.wal.Append(r)
}

// RecoverWAL replays recorded ranges using fn.
func (d *RawDevice) RecoverWAL(fn func(Range) error) error {
	if d.wal == nil {
		return nil
	}
	for _, r := range d.wal.Ranges() {
		if err := fn(r); err != nil {
			return err
		}
	}
	return nil
}

// Close closes the underlying file descriptor.
func (d *RawDevice) Close() error {
	err := d.f.Close()
	if err != nil {
		d.logger.Error("raw_device_close_failed", zap.String("path", d.Path()), zap.Error(err))
	} else {
		d.logger.Info("raw_device_closed", zap.String("path", d.Path()))
	}
	return err
}

// Snapshot returns the device itself for raw block devices.
func (d *RawDevice) Snapshot(context.Context, string) (Device, error) { return d, nil }

// Cleanup thaws the filesystem if a freeze command was issued.
func (d *RawDevice) Cleanup(ctx context.Context) error {
	if d.freezeIssued {
		if err := validateCmd(d.thawCmdPath, d.thawCmdArgs); err != nil {
			d.logger.Error("fs_thaw_failed", zap.Error(err))
			return fmt.Errorf("invalid thaw command: %w", err)
		}
		d.logger.Info("fs_thaw_start", zap.String("command", d.thawCmdPath), zap.Strings("args", d.thawCmdArgs))
		thawCtx := ctx
		if _, ok := ctx.Deadline(); !ok && d.thawTimeout > 0 {
			var cancel context.CancelFunc
			thawCtx, cancel = context.WithTimeout(ctx, d.thawTimeout)
			defer cancel()
		}
		out, cmdErr := d.runner.command.CommandContext(thawCtx, d.thawCmdPath, d.thawCmdArgs...).CombinedOutput()
		if cmdErr != nil {
			output := strings.TrimSpace(string(out))
			d.logger.Error("fs_thaw_failed", zap.Error(cmdErr), zap.String("output", output))
			return fmt.Errorf("thaw command failed: %w: %s", cmdErr, output)
		}
		d.logger.Info("fs_thaw_complete")
	}
	return nil
}

// validateCmd ensures the command path and arguments are suitable for execution.
func validateCmd(path string, args []string) error {
	if path == "" {
		return fmt.Errorf("command path is empty")
	}
	if !filepath.IsAbs(path) {
		return fmt.Errorf("command path must be absolute")
	}
	if strings.ContainsRune(path, '\x00') {
		return fmt.Errorf("command path contains NUL byte")
	}
	if !remote.ValidRemoteCommand(filepath.Base(path)) {
		return fmt.Errorf("command path %s contains invalid characters", path)
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
