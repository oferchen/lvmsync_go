package device

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const uuidTimeout = 5 * time.Second

var (
	uuidFunc  = defaultUUIDFunc
	mountFunc = defaultMountFunc
)

// SetUUIDFunc allows tests to override the implementation used to lookup a
// device's UUID or serial. It returns the previous function for restoration.
func SetUUIDFunc(f func(context.Context, string) (string, error)) func(context.Context, string) (string, error) {
	prev := uuidFunc
	uuidFunc = f
	return prev
}

// GetUUID returns the UUID or GPT serial of the device at path.
// If ctx has no deadline, a default timeout is applied.
func GetUUID(ctx context.Context, path string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, uuidTimeout)
		defer cancel()
	}
	return uuidFunc(ctx, path)
}

func defaultUUIDFunc(ctx context.Context, path string) (string, error) {
	out, err := exec.CommandContext(ctx, "blkid", "-o", "value", "-s", "UUID", path).Output()
	if err == nil && len(out) > 0 {
		return strings.TrimSpace(string(out)), nil
	}
	out, err = exec.CommandContext(ctx, "blkid", "-o", "value", "-s", "PARTUUID", path).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// SetMountFunc allows tests to override the mount status checker. It returns
// the previous function for restoration.
func SetMountFunc(f func(string) (bool, error)) func(string) (bool, error) {
	prev := mountFunc
	mountFunc = f
	return prev
}

// IsMountedRW reports whether the device at path is mounted read-write.
func IsMountedRW(path string) (bool, error) { return mountFunc(path) }

func defaultMountFunc(path string) (bool, error) {
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return false, err
	}
	f, err := os.Open("/proc/self/mounts")
	if err != nil {
		return false, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 4 {
			continue
		}
		if fields[0] == real {
			for _, opt := range strings.Split(fields[3], ",") {
				if opt == "rw" {
					return true, nil
				}
			}
			return false, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return false, err
	}
	return false, nil
}
