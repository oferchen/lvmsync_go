//go:build linux

package device

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"go.uber.org/multierr"
	"go.uber.org/zap"
	"golang.org/x/sys/unix"
	"golang.org/x/term"

	"lvmsync_go/internal/exitcode"
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
	wal         *WAL
}

// OpenLVM opens an LVM logical volume and queries its size and block size.
// It fails unless the device is a snapshot or offline is true. Size
// information is obtained through the lvm package helpers.
func (r *Runner) OpenLVM(ctx context.Context, path string, cache *lvm.FDCache, offline bool, escalation string, logger *zap.Logger) (*LVMDevice, error) {
	if r.openLVMOverride != nil {
		return r.openLVMOverride(ctx, path, cache, offline, escalation, logger)
	}
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
	mounted, err := r.isMountedRW(ctx, path)
	if err != nil {
		return nil, err
	}
	if mounted {
		return nil, fmt.Errorf("device %s mounted", path)
	}
	if !offline {
		if lvsErr != nil {
			return nil, lvsErr
		}
		out, err := exec.CommandContext(ctx, lvsPath, "--noheadings", "-o", "lv_attr,lv_role", path).Output()
		if err != nil {
			return nil, err
		}
		fields := strings.Fields(string(out))
		if len(fields) != 2 {
			return nil, fmt.Errorf("unexpected lvs output for %s", path)
		}
		attr, typ := fields[0], fields[1]
		if typ != "snapshot" && !strings.HasPrefix(attr, "s") {
			return nil, fmt.Errorf("precondition: volume %s is not a snapshot: %w", path, exitcode.ErrPrecondition)
		}
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

// Identity gathers size, device numbers and UUID info for the logical volume.
func (d *LVMDevice) Identity(ctx context.Context) (DeviceIdentity, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, identityTimeout)
		defer cancel()
	}
	if lvsErr != nil {
		return DeviceIdentity{}, lvsErr
	}
	if blkidErr != nil {
		return DeviceIdentity{}, blkidErr
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
	out, err := exec.CommandContext(ctx, lvsPath, "--noheadings", "-o", "lv_uuid", d.Path()).Output()
	if err != nil {
		if ctx.Err() != nil {
			return DeviceIdentity{}, ctx.Err()
		}
		return DeviceIdentity{}, err
	}
	id.KernelUUID = strings.TrimSpace(string(out))
	if out, err = exec.CommandContext(ctx, blkidPath, "-o", "value", "-s", "UUID", d.Path()).Output(); err != nil {
		if ctx.Err() != nil {
			return DeviceIdentity{}, ctx.Err()
		}
		return DeviceIdentity{}, err
	}
	id.FSUUID = strings.TrimSpace(string(out))
	if gpt, mbr, err := readPartitionSignatures(d.Path()); err == nil {
		id.GPTUUID = gpt
		id.MBRSignature = mbr
	}
	return id, nil
}

// SetWAL attaches a WAL to the device.
func (d *LVMDevice) SetWAL(w *WAL) { d.wal = w }

// AppendWAL records an applied range in the attached WAL.
func (d *LVMDevice) AppendWAL(r Range) error {
	if d.wal == nil {
		return nil
	}
	return d.wal.Append(r)
}

// RecoverWAL replays recorded ranges using fn.
func (d *LVMDevice) RecoverWAL(fn func(Range) error) error {
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

// Close closes the device and releases the lock if present.
func (d *LVMDevice) Close() error {
	var err error
	if d.f != nil {
		if cerr := d.f.Close(); cerr != nil {
			err = multierr.Append(err, cerr)
			d.logger.Error("lvm_device_close_failed", zap.String("path", d.path), zap.Error(cerr))
		} else {
			d.logger.Info("lvm_device_closed", zap.String("path", d.path))
		}
		d.f = nil
	}
	if d.lock != nil {
		if rerr := d.lock.Release(); rerr != nil {
			err = multierr.Append(err, rerr)
			d.logger.Error("lvm_device_lock_release_failed", zap.String("path", d.path), zap.Error(rerr))
		}
		d.lock = nil
	}
	return err
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
		parts, err := lvm.ParseEscalation(escalation)
		if err != nil {
			return err
		}
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

// CreateLV creates a new logical volume at path with the given size in bytes.
// The caller must provide ctx carrying --force and --allow-overwrite settings.
func (r *Runner) CreateLV(ctx context.Context, path string, size uint64, escalation string) error {
	if err := confirmOverwrite(ctx, os.Stdin, os.Stderr, term.IsTerminal); err != nil {
		return err
	}
	vg, lv, err := lvm.ParseLVPath(path)
	if err != nil {
		return err
	}
	if size == 0 {
		return fmt.Errorf("invalid size 0")
	}
	sizeStr := fmt.Sprintf("%dB", size)
	return r.runLVM(ctx, escalation, "lvcreate", "-n", lv, "-L", sizeStr, vg)
}

// Snapshot creates, activates, and opens an LVM snapshot of the device using
// the provided snapshotSize (e.g., "1G" or "20%").
func (d *LVMDevice) Snapshot(ctx context.Context, snapshotSize string) (Device, error) {
	if err := confirmOverwrite(ctx, os.Stdin, os.Stderr, term.IsTerminal); err != nil {
		return nil, err
	}
	vg, err := lvm.GetVolumeGroupName(d.path)
	if err != nil {
		return nil, err
	}
	snapName := generateSnapshot()
	if err := d.runner.runLVM(ctx, d.escalation, "lvcreate", "-s", "-n", snapName, "-L", snapshotSize, "-pr", d.path); err != nil {
		return nil, err
	}
	snapPath := lvm.GetSnapshotDevicePath(snapName, vg, d.logger)
	if err := d.runner.runLVM(ctx, d.escalation, "lvchange", "-ay", "-pr", snapPath); err != nil {
		_ = d.runner.runLVM(ctx, d.escalation, "lvremove", "-f", snapPath)
		return nil, err
	}
	cache, err := lvm.NewDeviceFDCache(d.logger)
	if err != nil {
		_ = d.runner.runLVM(ctx, d.escalation, "lvremove", "-f", snapPath)
		return nil, err
	}
	defer cache.Close()
	snapDev, err := d.runner.OpenLVM(ctx, snapPath, cache, false, d.escalation, d.logger)
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
