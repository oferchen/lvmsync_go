// lvm/lvm.go
package lvm

import (
	"container/list"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/dustin/go-humanize"
	"go.uber.org/zap"
	"golang.org/x/sys/unix"
)

const (
	BLKGETSIZE64 = 0x80081272

	fdCacheSize = 16
)

var (
	escalationCommand     string
	escalationCommandLock sync.RWMutex
)

type fdCacheEntry struct {
	path string
	fd   int
}

type fdCache struct {
	fds   map[string]*list.Element
	order *list.List
	mutex sync.Mutex
}

var deviceFDCache = &fdCache{
	fds:   make(map[string]*list.Element),
	order: list.New(),
}

var statfsFunc = unix.Statfs

var checkPrivs = checkRootPrivileges

func ioctlGetUint64(fd int, req uint) (uint64, error) {
	var value uint64
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), uintptr(req), uintptr(unsafe.Pointer(&value)))
	if errno != 0 {
		return 0, errno
	}
	return value, nil
}

func SetEscalationCommand(cmd string) {
	escalationCommandLock.Lock()
	defer escalationCommandLock.Unlock()
	escalationCommand = cmd
}

func GetEscalationCommand() string {
	escalationCommandLock.RLock()
	defer escalationCommandLock.RUnlock()
	return escalationCommand
}

func checkRootPrivileges() error {
	if os.Geteuid() != 0 && GetEscalationCommand() == "" {
		return fmt.Errorf("insufficient privileges: LVM operations require root privileges")
	}
	return nil
}

func (c *fdCache) getFD(devicePath string) (int, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if elem, exists := c.fds[devicePath]; exists {
		c.order.MoveToFront(elem)
		return elem.Value.(*fdCacheEntry).fd, nil
	}

	fd, err := unix.Open(devicePath, unix.O_RDONLY|unix.O_NONBLOCK, 0)
	if err != nil {
		return -1, fmt.Errorf("failed to open device %s: %w", devicePath, err)
	}

	if c.order.Len() >= fdCacheSize {
		back := c.order.Back()
		if back != nil {
			entry := back.Value.(*fdCacheEntry)
			unix.Close(entry.fd)
			delete(c.fds, entry.path)
			c.order.Remove(back)
		}
	}

	entry := &fdCacheEntry{path: devicePath, fd: fd}
	elem := c.order.PushFront(entry)
	c.fds[devicePath] = elem
	return fd, nil
}

func (c *fdCache) Close() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	for _, elem := range c.fds {
		entry := elem.Value.(*fdCacheEntry)
		unix.Close(entry.fd)
	}
	c.fds = make(map[string]*list.Element)
	c.order.Init()
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

func GetVolumeSize(volumePath string) (uint64, error) {
	fd, err := deviceFDCache.getFD(volumePath)
	if err != nil {
		return 0, err
	}

	size, err := ioctlGetUint64(fd, BLKGETSIZE64)
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

func GetVolumeAttributes(volumePath string) (map[string]string, error) {
	devName := filepath.Base(volumePath)
	sysfsPath := filepath.Join(sysBlockPath, devName)

	if _, err := os.Stat(sysfsPath); err != nil {
		return nil, fmt.Errorf("device %s not found in sysfs: %w", devName, err)
	}

	attributes := make(map[string]string)

	attrFiles := []string{"dev", "size", "ro", "removable"}
	for _, attr := range attrFiles {
		data, err := os.ReadFile(filepath.Join(sysfsPath, attr))
		if err != nil {
			zap.L().Warn("Failed to read attribute",
				zap.String("device", devName),
				zap.String("attribute", attr),
				zap.Error(err))
			continue
		}
		attributes[attr] = strings.TrimSpace(string(data))
	}

	return attributes, nil
}

func ParseSnapshotSize(sizeStr, volumePath string) (uint64, error) {
	sizeStr = strings.TrimSpace(sizeStr)

	if strings.HasSuffix(sizeStr, "%") {
		percentStr := strings.TrimSuffix(sizeStr, "%")
		percent, err := strconv.ParseFloat(percentStr, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid percentage value %q: %w", sizeStr, err)
		}

		if percent <= 0 || percent > 100 {
			return 0, fmt.Errorf("percentage must be between 0 and 100, got %v", percent)
		}

		volSize, err := GetVolumeSize(volumePath)
		if err != nil {
			return 0, fmt.Errorf("failed to get volume size for %q: %w", volumePath, err)
		}

		parsedSize := uint64(float64(volSize) * (percent / 100.0))

		zap.L().Debug("Parsed snapshot size from percentage",
			zap.String("input", sizeStr),
			zap.Uint64("calculated_bytes", parsedSize))

		return parsedSize, nil
	}

	parsedSize, err := humanize.ParseBytes(sizeStr)
	if err != nil {
		return 0, fmt.Errorf("failed to parse snapshot size %q: %w", sizeStr, err)
	}

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

// SelectVolumeGroupByFreeSpace chooses the volume group with the most free space.
// If candidates is non-empty, only those volume groups are considered.
func SelectVolumeGroupByFreeSpace(ctx context.Context, candidates []string) (VolumeGroup, error) {
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
	chosen := vgs[0]
	for _, vg := range vgs[1:] {
		if vg.Free > chosen.Free {
			chosen = vg
		}
	}
	return chosen, nil
}

func Cleanup() {
	deviceFDCache.Close()
}
