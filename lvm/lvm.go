// lvm/lvm.go
package lvm

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/dustin/go-humanize"
	"go.uber.org/zap"
)

const BLKGETSIZE64 = 0x80081272

var EscalationCommand = "sudo -n"

func SetEscalationCommand(cmd string) {
	EscalationCommand = cmd
}

func ensurePrivileges() (bool, error) {
	if os.Geteuid() == 0 {
		return true, nil
	}
	parts := strings.Fields(EscalationCommand)
	if len(parts) == 0 {
		return false, fmt.Errorf("escalation command is empty")
	}
	if _, err := exec.LookPath(parts[0]); err == nil {
		return false, nil
	}
	return false, fmt.Errorf("insufficient privileges: not running as root and escalation command %q not available", EscalationCommand)
}

func CreateSnapshot(lvPath, snapshotName, size string) error {
	isRoot, err := ensurePrivileges()
	if err != nil {
		return err
	}

	var cmd *exec.Cmd
	if isRoot {
		cmd = exec.Command("lvcreate", "-L", size, "-s", "-n", snapshotName, lvPath)
	} else {
		prefix := strings.Fields(EscalationCommand)
		args := append(prefix, "lvcreate", "-L", size, "-s", "-n", snapshotName, lvPath)
		cmd = exec.Command(args[0], args[1:]...)
	}

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create snapshot: %v, output: %s", err, string(out))
	}
	return nil
}

func RemoveSnapshot(snapshotPath string) error {
	isRoot, err := ensurePrivileges()
	if err != nil {
		return err
	}

	var cmd *exec.Cmd
	if isRoot {
		cmd = exec.Command("lvremove", "-f", snapshotPath)
	} else {
		prefix := strings.Fields(EscalationCommand)
		args := append(prefix, "lvremove", "-f", snapshotPath)
		cmd = exec.Command(args[0], args[1:]...)
	}

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to remove snapshot: %v, output: %s", err, string(out))
	}
	return nil
}

func CheckDiskSpace(mountPoint string) (uint64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(mountPoint, &stat); err != nil {
		return 0, fmt.Errorf("failed to get disk stats for %q: %v", mountPoint, err)
	}
	return stat.Bavail * uint64(stat.Bsize), nil
}

func GetVolumeSize(volumePath string) (uint64, error) {
	f, err := os.Open(volumePath)
	if err != nil {
		return 0, fmt.Errorf("failed to open volume %q: %v", volumePath, err)
	}
	defer f.Close()

	var size uint64
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), uintptr(BLKGETSIZE64), uintptr(unsafe.Pointer(&size)))
	if errno != 0 {
		return 0, fmt.Errorf("ioctl BLKGETSIZE64 failed on %q: %v", volumePath, errno)
	}
	return size, nil
}

func ParseSnapshotSize(sizeStr, volumePath string) (uint64, error) {
	sizeStr = strings.TrimSpace(sizeStr)
	if strings.HasSuffix(sizeStr, "%") {
		percentStr := strings.TrimSuffix(sizeStr, "%")
		percent, err := strconv.ParseFloat(percentStr, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid percentage value %q: %v", sizeStr, err)
		}
		if percent <= 0 || percent > 100 {
			return 0, fmt.Errorf("percentage must be between 0 and 100, got %v", percent)
		}
		volSize, err := GetVolumeSize(volumePath)
		if err != nil {
			return 0, fmt.Errorf("failed to get volume size: %v", err)
		}
		return uint64(float64(volSize) * (percent / 100.0)), nil
	}
	return humanize.ParseBytes(sizeStr)
}

func GetSnapshotDevicePath(snapshotName, volumeGroup string) string {
	return fmt.Sprintf("/dev/%s/%s", volumeGroup, snapshotName)
}

func MonitorSnapshot(snapshotPath string, threshold float64, interval time.Duration, stopChan <-chan struct{}) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			out, err := exec.Command("lvs", "--noheadings", "-o", "lv_used_percent", "--units", "%", "--nosuffix", snapshotPath).Output()
			if err != nil {
				return fmt.Errorf("failed to get snapshot usage: %v", err)
			}
			usageStr := strings.TrimSpace(string(out))
			usage, err := strconv.ParseFloat(usageStr, 64)
			if err != nil {
				return fmt.Errorf("failed to parse snapshot usage %q: %v", usageStr, err)
			}
			zap.L().Info("Snapshot usage", zap.Float64("usage_percent", usage))
			if usage >= threshold {
				return fmt.Errorf("snapshot usage (%.2f%%) exceeds threshold (%.2f%%)", usage, threshold)
			}
		case <-stopChan:
			return nil
		}
	}
}
