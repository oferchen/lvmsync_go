// lvm/lvm.go
package lvm

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
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

type fdCache struct {
	fds   map[string]int
	mutex sync.Mutex
}

var deviceFDCache = &fdCache{
	fds: make(map[string]int),
}

var statfsFunc = syscall.Statfs

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

func buildCommand(name string, args ...string) *exec.Cmd {
	escalationCommandLock.RLock()
	escCmd := escalationCommand
	escalationCommandLock.RUnlock()

	if escCmd != "" && os.Geteuid() != 0 {
		cmdArgs := append([]string{name}, args...)
		return exec.Command(escCmd, cmdArgs...)
	}

	return exec.Command(name, args...)
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

	if fd, exists := c.fds[devicePath]; exists {
		return fd, nil
	}

	fd, err := syscall.Open(devicePath, syscall.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return -1, fmt.Errorf("failed to open device %s: %w", devicePath, err)
	}

	if len(c.fds) >= fdCacheSize {
		var oldestPath string
		for path := range c.fds {
			oldestPath = path
			break
		}
		syscall.Close(c.fds[oldestPath])
		delete(c.fds, oldestPath)
	}

	c.fds[devicePath] = fd
	return fd, nil
}

func (c *fdCache) Close() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	for path, fd := range c.fds {
		syscall.Close(fd)
		delete(c.fds, path)
	}
}

func captureOutput(r io.Reader, ch chan<- error, buf *bytes.Buffer) {
	_, err := buf.ReadFrom(r)
	ch <- err
}

func executeCommand(name string, args ...string) ([]byte, error) {
	cmd := buildCommand(name, args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	var outBuf, errBuf bytes.Buffer
	outCh := make(chan error, 1)
	errCh := make(chan error, 1)

	go captureOutput(stdout, outCh, &outBuf)
	go captureOutput(stderr, errCh, &errBuf)

	<-outCh
	<-errCh

	err = cmd.Wait()
	if err != nil {
		return outBuf.Bytes(), fmt.Errorf("%w: %s", err, errBuf.String())
	}

	return outBuf.Bytes(), nil
}

func CreateSnapshot(lvPath, snapshotName, size string) error {
	if err := checkRootPrivileges(); err != nil {
		return err
	}

	if lvPath == "" || snapshotName == "" || size == "" {
		return fmt.Errorf("invalid parameters: lvPath, snapshotName, and size must be non-empty")
	}

	output, err := executeCommand("lvcreate", "-s", "-n", snapshotName, "-L", size, lvPath)
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
	if err := checkRootPrivileges(); err != nil {
		return err
	}

	if snapshotPath == "" {
		return fmt.Errorf("invalid parameter: snapshotPath must be non-empty")
	}

	output, err := executeCommand("lvremove", "-f", snapshotPath)
	if err != nil {
		return fmt.Errorf("failed to remove snapshot [%s]: %w", snapshotPath, err)
	}

	zap.L().Info("Snapshot removed successfully",
		zap.String("snapshot_path", snapshotPath),
		zap.String("output", string(output)))

	return nil
}

func GetSnapshotUsage(snapshotPath string) (float64, error) {
	if err := checkRootPrivileges(); err != nil {
		return 0, err
	}

	if snapshotPath == "" {
		return 0, fmt.Errorf("invalid parameter: snapshotPath must be non-empty")
	}

	output, err := executeCommand("lvs", "--noheadings", "--units", "b",
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
	if err := checkRootPrivileges(); err != nil {
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
	if err := checkRootPrivileges(); err != nil {
		return 0, err
	}

	output, err := executeCommand("vgs", "--noheadings", "--units", "b",
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

func Cleanup() {
	deviceFDCache.Close()
}
