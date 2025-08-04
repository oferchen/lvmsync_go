// lvm/lvm.go
package lvm

/*
#cgo LDFLAGS: -llvm2cmd
#include <stdlib.h>
#include <lvm2cmd.h>

extern void goLvmLog(int level, char *file, int line, int dm_errno, char *msg);
static inline void setLvmLog() { lvm2_log_fn((lvm2_log_fn_t)goLvmLog); }
*/
import "C"

import (
	"bytes"
	"container/list"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/dustin/go-humanize"
	"go.uber.org/zap"
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

var statfsFunc = syscall.Statfs

var checkPrivs = checkRootPrivileges

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
	escalationCommandLock.RLock()
	escCmd := escalationCommand
	escalationCommandLock.RUnlock()

	if os.Geteuid() != 0 && escCmd == "" {
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

	fd, err := syscall.Open(devicePath, syscall.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return -1, fmt.Errorf("failed to open device %s: %w", devicePath, err)
	}

	if c.order.Len() >= fdCacheSize {
		back := c.order.Back()
		if back != nil {
			entry := back.Value.(*fdCacheEntry)
			syscall.Close(entry.fd)
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
		syscall.Close(entry.fd)
	}
	c.fds = make(map[string]*list.Element)
	c.order.Init()
}

var (
	lvmHandle     = C.lvm2_init()
	cmdMutex      sync.Mutex
	logBuffer     *bytes.Buffer
	runLVMCommand = realRunLVMCommand
)

// SetRunLVMCommand overrides the function used to execute LVM commands.
// It returns a restore function to reset the original behavior.
func SetRunLVMCommand(f func(string, ...string) ([]byte, error)) func() {
	orig := runLVMCommand
	if f == nil {
		runLVMCommand = realRunLVMCommand
	} else {
		runLVMCommand = f
	}
	return func() { runLVMCommand = orig }
}

func init() {
	C.setLvmLog()
	C.lvm2_log_level(lvmHandle, C.LVM2_LOG_DEBUG)
}

//export goLvmLog
func goLvmLog(level C.int, file *C.char, line C.int, dm_errno C.int, msg *C.char) {
	cmdMutex.Lock()
	if logBuffer != nil {
		logBuffer.WriteString(C.GoString(msg))
		logBuffer.WriteByte('\n')
	}
	cmdMutex.Unlock()
}

func realRunLVMCommand(name string, args ...string) ([]byte, error) {
	cmdMutex.Lock()
	defer cmdMutex.Unlock()

	logBuffer = &bytes.Buffer{}
	var b strings.Builder
	b.WriteString(name)
	for _, a := range args {
		b.WriteByte(' ')
		if strings.ContainsAny(a, " \t\n\"") {
			b.WriteString(strconv.Quote(a))
		} else {
			b.WriteString(a)
		}
	}
	ccmd := C.CString(b.String())
	defer C.free(unsafe.Pointer(ccmd))

	ret := C.lvm2_run(lvmHandle, ccmd)
	out := logBuffer.Bytes()
	logBuffer = nil
	if ret != C.LVM2_COMMAND_SUCCEEDED {
		return out, fmt.Errorf("lvm command failed: %s", strings.TrimSpace(string(out)))
	}
	return out, nil
}

func CreateSnapshot(lvPath, snapshotName, size string) error {
	if err := checkPrivs(); err != nil {
		return err
	}

	if lvPath == "" || snapshotName == "" || size == "" {
		return fmt.Errorf("invalid parameters: lvPath, snapshotName, and size must be non-empty")
	}

	output, err := runLVMCommand("lvcreate", "-s", "-n", snapshotName, "-L", size, lvPath)
	if err != nil {
		return fmt.Errorf("failed to create snapshot [%s] for LV %s with size %s: %w",
			snapshotName, lvPath, size, err)
	}

	zap.L().Info("Snapshot created successfully",
		zap.String("lv_path", lvPath),
		zap.String("snapshot_name", snapshotName),
		zap.String("size", size),
		zap.String("output", string(output)))

	return nil
}

func RemoveSnapshot(snapshotPath string) error {
	if err := checkPrivs(); err != nil {
		return err
	}

	if snapshotPath == "" {
		return fmt.Errorf("invalid parameter: snapshotPath must be non-empty")
	}

	output, err := runLVMCommand("lvremove", "-f", snapshotPath)
	if err != nil {
		return fmt.Errorf("failed to remove snapshot [%s]: %w", snapshotPath, err)
	}

	zap.L().Info("Snapshot removed successfully",
		zap.String("snapshot_path", snapshotPath),
		zap.String("output", string(output)))

	return nil
}

func GetSnapshotUsage(snapshotPath string) (float64, error) {
	if err := checkPrivs(); err != nil {
		return 0, err
	}

	if snapshotPath == "" {
		return 0, fmt.Errorf("invalid parameter: snapshotPath must be non-empty")
	}

	output, err := runLVMCommand("lvs", "--noheadings", "--units", "b",
		"--options", "data_percent", snapshotPath)
	if err != nil {
		return 0, fmt.Errorf("failed to get snapshot usage for %s: %w", snapshotPath, err)
	}

	usageStr := strings.TrimSpace(string(output))
	usage, err := strconv.ParseFloat(usageStr, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse snapshot usage %q: %w", usageStr, err)
	}

	zap.L().Info("Snapshot usage retrieved",
		zap.String("snapshot", snapshotPath),
		zap.Float64("usage_percent", usage))

	return usage, nil
}

func MonitorSnapshot(snapshotPath string, threshold float64, interval time.Duration, stopChan <-chan struct{}) error {
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
			usage, err := GetSnapshotUsage(snapshotPath)
			if err != nil {
				return err
			}

			if usage >= threshold {
				return fmt.Errorf("snapshot usage (%.2f%%) exceeds threshold (%.2f%%)", usage, threshold)
			}
		case <-stopChan:
			zap.L().Info("Snapshot monitoring stopped", zap.String("snapshot", snapshotPath))
			return nil
		}
	}
}

func CheckDiskSpace(mountPoint string) (uint64, error) {
	var stat syscall.Statfs_t
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

	var size uint64
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), uintptr(BLKGETSIZE64), uintptr(unsafe.Pointer(&size)))
	if errno != 0 {
		if errno == syscall.ENOTTY {
			info, statErr := os.Stat(volumePath)
			if statErr != nil {
				return 0, fmt.Errorf("stat failed on %q: %w", volumePath, statErr)
			}
			size = uint64(info.Size())
		} else {
			return 0, fmt.Errorf("ioctl BLKGETSIZE64 failed on %q: %w", volumePath, errno)
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

func GetVolumeGroupFreeSpace(vgName string) (uint64, error) {
	if err := checkPrivs(); err != nil {
		return 0, err
	}

	output, err := runLVMCommand("vgs", "--noheadings", "--units", "b",
		"--options", "vg_free", vgName)
	if err != nil {
		return 0, fmt.Errorf("failed to get free space for VG %s: %w", vgName, err)
	}

	sizeStr := strings.TrimSpace(string(output))
	sizeStr = strings.TrimSuffix(sizeStr, "B")
	sizeStr = strings.TrimSpace(sizeStr)

	size, err := strconv.ParseUint(sizeStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse VG free space %q: %w", sizeStr, err)
	}

	return size, nil
}

// VolumeGroup represents basic information about a volume group.
type VolumeGroup struct {
	Name string
	Free uint64
}

// ListVolumeGroups returns information about all available volume groups.
func ListVolumeGroups() ([]VolumeGroup, error) {
	if err := checkPrivs(); err != nil {
		return nil, err
	}

	output, err := runLVMCommand("vgs", "--noheadings", "--units", "b",
		"--options", "vg_name,vg_free", "--separator", ":")
	if err != nil {
		return nil, fmt.Errorf("failed to list volume groups: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	vgs := make([]VolumeGroup, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.Split(line, ":")
		if len(parts) != 2 {
			continue
		}
		name := strings.TrimSpace(parts[0])
		freeStr := strings.TrimSuffix(strings.TrimSpace(parts[1]), "B")
		free, err := strconv.ParseUint(freeStr, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("failed to parse VG free space %q: %w", freeStr, err)
		}
		vgs = append(vgs, VolumeGroup{Name: name, Free: free})
	}
	return vgs, nil
}

// SelectVolumeGroupByFreeSpace chooses the volume group with the most free space.
// If candidates is non-empty, only those volume groups are considered.
func SelectVolumeGroupByFreeSpace(candidates []string) (string, uint64, error) {
	vgs, err := ListVolumeGroups()
	if err != nil {
		return "", 0, err
	}

	candidateSet := make(map[string]struct{}, len(candidates))
	for _, c := range candidates {
		candidateSet[c] = struct{}{}
	}

	var chosen VolumeGroup
	for _, vg := range vgs {
		if len(candidateSet) > 0 {
			if _, ok := candidateSet[vg.Name]; !ok {
				continue
			}
		}
		if vg.Free > chosen.Free {
			chosen = vg
		}
	}
	if chosen.Name == "" {
		if len(candidateSet) > 0 {
			return "", 0, fmt.Errorf("no matching volume group found")
		}
		return "", 0, fmt.Errorf("no volume groups found")
	}
	return chosen.Name, chosen.Free, nil
}

func Cleanup() {
	deviceFDCache.Close()
}
