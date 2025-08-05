// lvm/lvm.go
package lvm

import (
	"context"
	"fmt"
	"lvmsync_go/internal/sizeparse"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"go.uber.org/zap"
	"golang.org/x/sys/unix"
)

const (
	BLKGETSIZE64 = 0x80081272
)

var statfsFunc = unix.Statfs

var checkPrivs = checkRootPrivileges

var ioctlGetUint64Func = ioctlGetUint64

func checkRootPrivileges() error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("insufficient privileges: LVM operations require root privileges")
	}
	return nil
}

// SetPrivilegeChecker overrides the default privilege check function. It
// returns a restore function to reset the original behavior.
func SetPrivilegeChecker(fn func() error) func() {
	orig := checkPrivs
	if fn == nil {
		checkPrivs = checkRootPrivileges
	} else {
		checkPrivs = fn
	}
	return func() { checkPrivs = orig }
}

// backend is used to execute LVM operations. It can be overridden for tests.
var backend lvmBackend = newLVMBackend()

// SetBackend overrides the LVM backend. It returns a restore function to reset the original behavior.
func SetBackend(b lvmBackend) func() {
	orig := backend
	if b == nil {
		backend = newLVMBackend()
	} else {
		backend = b
	}
	return func() { backend = orig }
}

func CreateSnapshot(ctx context.Context, lvPath, snapshotName, size string) error {
	if err := checkPrivs(); err != nil {
		return err
	}

	if lvPath == "" || snapshotName == "" || size == "" {
		return fmt.Errorf("invalid parameters: lvPath, snapshotName, and size must be non-empty")
	}

	if err := backend.CreateSnapshot(ctx, lvPath, snapshotName, size); err != nil {
		return fmt.Errorf("failed to create snapshot [%s] for LV %s with size %s: %w",
			snapshotName, lvPath, size, err)
	}

	zap.L().Info("Snapshot created successfully",
		zap.String("lv_path", lvPath),
		zap.String("snapshot_name", snapshotName),
		zap.String("size", size))

	return nil
}

func RemoveSnapshot(ctx context.Context, snapshotPath string) error {
	if err := checkPrivs(); err != nil {
		return err
	}

	if snapshotPath == "" {
		return fmt.Errorf("invalid parameter: snapshotPath must be non-empty")
	}

	if err := backend.RemoveSnapshot(ctx, snapshotPath); err != nil {
		return fmt.Errorf("failed to remove snapshot [%s]: %w", snapshotPath, err)
	}

	zap.L().Info("Snapshot removed successfully",
		zap.String("snapshot_path", snapshotPath))

	return nil
}

func GetSnapshotUsage(ctx context.Context, snapshotPath string) (float64, error) {
	if err := checkPrivs(); err != nil {
		return 0, err
	}

	if snapshotPath == "" {
		return 0, fmt.Errorf("invalid parameter: snapshotPath must be non-empty")
	}

	usage, err := backend.GetSnapshotUsage(ctx, snapshotPath)
	if err != nil {
		return 0, fmt.Errorf("failed to get snapshot usage for %s: %w", snapshotPath, err)
	}

	zap.L().Info("Snapshot usage retrieved",
		zap.String("snapshot", snapshotPath),
		zap.Float64("usage_percent", usage))

	return usage, nil
}

func MonitorSnapshot(ctx context.Context, snapshotPath string, threshold float64, interval time.Duration) error {
	if err := checkPrivs(); err != nil {
		return err
	}

	if snapshotPath == "" {
		return fmt.Errorf("invalid parameter: snapshotPath must be non-empty")
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			usage, err := GetSnapshotUsage(ctx, snapshotPath)
			if err != nil {
				return err
			}

			if usage >= threshold {
				return fmt.Errorf("snapshot usage (%.2f%%) exceeds threshold (%.2f%%)", usage, threshold)
			}
		case <-ctx.Done():
			zap.L().Info("Snapshot monitoring stopped", zap.String("snapshot", snapshotPath))
			return ctx.Err()
		}
	}
}

func CheckDiskSpace(mountPoint string) (uint64, error) {
	var stat unix.Statfs_t
	if err := statfsFunc(mountPoint, &stat); err != nil {
		return 0, fmt.Errorf("failed to get disk stats for %q: %w", mountPoint, err)
	}

	available := stat.Bavail * uint64(stat.Bsize)
	zap.L().Debug("Disk space check",
		zap.String("mount_point", mountPoint),
		zap.Uint64("available_bytes", available))

	return available, nil
}

func ioctlGetUint64(fd int, req uint) (uint64, error) {
	var value uint64
	_, _, err := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), uintptr(req), uintptr(unsafe.Pointer(&value)))
	if err != 0 {
		return 0, err
	}
	return value, nil
}

func GetVolumeSize(volumePath string) (uint64, error) {
	fd, err := deviceFDCache.getFD(volumePath)
	if err != nil {
		return 0, err
	}

	size, err := ioctlGetUint64Func(fd, BLKGETSIZE64)
	if err != nil {
		if err == unix.ENOTTY {
			info, statErr := os.Stat(volumePath)
			if statErr != nil {
				return 0, fmt.Errorf("stat failed on %q: %w", volumePath, statErr)
			}
			size = uint64(info.Size())
		} else {
			return 0, fmt.Errorf("ioctl BLKGETSIZE64 failed on %q: %w", volumePath, err)
		}
	}

	zap.L().Debug("Volume size retrieved",
		zap.String("volume_path", volumePath),
		zap.Uint64("size_bytes", size))

	return size, nil
}

var sysBlockPath = "/sys/block"

func SetSysBlockPath(path string) {
	sysBlockPath = path
}

type VolumeAttributes struct {
	Major     int
	Minor     int
	Size      uint64
	ReadOnly  bool
	Removable bool
}

func readUintAttr(sysfsPath, name string) (uint64, error) {
	path := filepath.Join(sysfsPath, name)
	data, err := os.ReadFile(path)
	if err != nil {
		zap.L().Warn("Failed to read attribute",
			zap.String("device", filepath.Base(sysfsPath)),
			zap.String("attribute", name),
			zap.Error(err))
		return 0, err
	}

	val, err := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)
	if err != nil {
		zap.L().Warn("Failed to parse attribute",
			zap.String("device", filepath.Base(sysfsPath)),
			zap.String("attribute", name),
			zap.Error(err))
		return 0, err
	}
	return val, nil
}

func readBoolAttr(sysfsPath, name string) (bool, error) {
	val, err := readUintAttr(sysfsPath, name)
	if err != nil {
		return false, err
	}

	switch val {
	case 0:
		return false, nil
	case 1:
		return true, nil
	default:
		err := fmt.Errorf("invalid boolean value %d", val)
		zap.L().Warn("Invalid boolean attribute",
			zap.String("device", filepath.Base(sysfsPath)),
			zap.String("attribute", name),
			zap.Error(err))
		return false, err
	}
}

func GetVolumeAttributes(volumePath string) (*VolumeAttributes, error) {
	devName := filepath.Base(volumePath)
	sysfsPath := filepath.Join(sysBlockPath, devName)

	if _, err := os.Stat(sysfsPath); err != nil {
		return nil, fmt.Errorf("device %s not found in sysfs: %w", devName, err)
	}

	attrs := &VolumeAttributes{}

	// dev: major:minor
	if data, err := os.ReadFile(filepath.Join(sysfsPath, "dev")); err == nil {
		parts := strings.Split(strings.TrimSpace(string(data)), ":")
		if len(parts) == 2 {
			if major, err := strconv.Atoi(parts[0]); err == nil {
				attrs.Major = major
			}
			if minor, err := strconv.Atoi(parts[1]); err == nil {
				attrs.Minor = minor
			}
		}
	} else {
		zap.L().Warn("Failed to read attribute",
			zap.String("device", devName),
			zap.String("attribute", "dev"),
			zap.Error(err))
	}

	// size
	if size, err := readUintAttr(sysfsPath, "size"); err == nil {
		attrs.Size = size
	}

	// read-only flag
	if ro, err := readBoolAttr(sysfsPath, "ro"); err == nil {
		attrs.ReadOnly = ro
	}

	// removable flag
	if removable, err := readBoolAttr(sysfsPath, "removable"); err == nil {
		attrs.Removable = removable
	}

	return attrs, nil
}

func ParseSnapshotSize(sizeStr, volumePath string) (uint64, error) {
	sizeStr = strings.TrimSpace(sizeStr)

	val, isPercent, err := sizeparse.Parse(sizeStr)
	if err != nil {
		return 0, fmt.Errorf("failed to parse snapshot size %q: %w", sizeStr, err)
	}

	if isPercent {
		if val <= 0 || val > 100 {
			return 0, fmt.Errorf("percentage must be between 0 and 100, got %v", val)
		}

		volSize, err := GetVolumeSize(volumePath)
		if err != nil {
			return 0, fmt.Errorf("failed to get volume size for %q: %w", volumePath, err)
		}

		parsedSize := uint64(float64(volSize) * (val / 100.0))

		zap.L().Debug("Parsed snapshot size from percentage",
			zap.String("input", sizeStr),
			zap.Uint64("calculated_bytes", parsedSize))

		return parsedSize, nil
	}

	parsedSize := uint64(val)

	zap.L().Debug("Parsed snapshot size",
		zap.String("input", sizeStr),
		zap.Uint64("bytes", parsedSize))

	return parsedSize, nil
}

func GetSnapshotDevicePath(snapshotName, volumeGroup string) string {
	path := fmt.Sprintf("/dev/%s/%s", volumeGroup, snapshotName)
	zap.L().Debug("Constructed snapshot device path", zap.String("path", path))
	return path
}

func GetVolumeGroupFreeSpace(ctx context.Context, vgName string) (uint64, error) {
	if err := checkPrivs(); err != nil {
		return 0, err
	}

	size, err := backend.GetVolumeGroupFreeSpace(ctx, vgName)
	if err != nil {
		return 0, fmt.Errorf("failed to get free space for VG %s: %w", vgName, err)
	}
	return size, nil
}

// VolumeGroup represents basic information about a volume group.
type VolumeGroup struct {
	Name string
	Free uint64
}

// ListVolumeGroups returns information about all available volume groups.
func ListVolumeGroups(ctx context.Context) ([]VolumeGroup, error) {
	if err := checkPrivs(); err != nil {
		return nil, err
	}

	vgs, err := backend.ListVolumeGroups(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list volume groups: %w", err)
	}
	return vgs, nil
}

// VolumeGroupSelector defines the strategy used to choose a volume group
// from a list of candidates.
type VolumeGroupSelector func([]VolumeGroup) (VolumeGroup, error)

// SelectVolumeGroup chooses a volume group from the system using the provided
// selector strategy. If candidates is non-empty, only those volume groups are
// considered.
func SelectVolumeGroup(ctx context.Context, candidates []string, selector VolumeGroupSelector) (VolumeGroup, error) {
	if selector == nil {
		return VolumeGroup{}, fmt.Errorf("selector must not be nil")
	}
	if err := checkPrivs(); err != nil {
		return VolumeGroup{}, err
	}
	vgs, err := backend.ListVolumeGroups(ctx, candidates)
	if err != nil {
		return VolumeGroup{}, err
	}
	if len(vgs) == 0 {
		if len(candidates) > 0 {
			return VolumeGroup{}, fmt.Errorf("no matching volume group found")
		}
		return VolumeGroup{}, fmt.Errorf("no volume groups found")
	}
	return selector(vgs)
}

// ByFreeSpace selects the volume group with the largest amount of free space.
func ByFreeSpace(vgs []VolumeGroup) (VolumeGroup, error) {
	chosen := vgs[0]
	for _, vg := range vgs[1:] {
		if vg.Free > chosen.Free {
			chosen = vg
		}
	}
	return chosen, nil
}

// ByFreeSpaceFit returns a selector that chooses the volume group with the
// largest amount of free space among those providing at least the required
// amount of free bytes. It fails if no volume group satisfies the requirement.
func ByFreeSpaceFit(required uint64) VolumeGroupSelector {
	return func(vgs []VolumeGroup) (VolumeGroup, error) {
		var (
			chosen VolumeGroup
			found  bool
		)
		for _, vg := range vgs {
			if vg.Free < required {
				continue
			}
			if !found || vg.Free > chosen.Free {
				chosen = vg
				found = true
			}
		}
		if !found {
			return VolumeGroup{}, fmt.Errorf("no volume group has %d bytes free", required)
		}
		return chosen, nil
	}
}

// SelectVolumeGroupForSize selects a volume group from the given candidates
// that has at least the required amount of free space. If required is zero it
// behaves like SelectVolumeGroup with ByFreeSpace.
func SelectVolumeGroupForSize(ctx context.Context, candidates []string, required uint64) (VolumeGroup, error) {
	selector := ByFreeSpace
	if required > 0 {
		selector = ByFreeSpaceFit(required)
	}
	return SelectVolumeGroup(ctx, candidates, selector)
}

func Cleanup() {
	deviceFDCache.Close()
}
