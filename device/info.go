package device

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/moby/sys/mountinfo"
	"go.uber.org/zap"
)

const uuidTimeout = 5 * time.Second

var (
	uuidFunc    = defaultUUIDFunc
	lvmUUIDFunc = defaultLVMUUIDFunc
	mountFunc   = defaultMountFunc
)

// SetUUIDFunc allows tests to override the implementation used to lookup a
// device's UUID or serial. It returns the previous function for restoration.
func SetUUIDFunc(f func(context.Context, string) (string, error)) func(context.Context, string) (string, error) {
	prev := uuidFunc
	uuidFunc = f
	return prev
}

// SetLVMUUIDFunc allows tests to override the implementation used to lookup an
// LVM logical volume's UUID. It returns the previous function for restoration.
func SetLVMUUIDFunc(f func(context.Context, string) (string, error)) func(context.Context, string) (string, error) {
	prev := lvmUUIDFunc
	lvmUUIDFunc = f
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

// GetDeviceID returns the LVM logical volume UUID if available, falling back to
// the device's blkid/GPT serial. If ctx has no deadline, a default timeout is
// applied.
func GetDeviceID(ctx context.Context, path string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, uuidTimeout)
		defer cancel()
	}
	if id, err := lvmUUIDFunc(ctx, path); err == nil && id != "" {
		return id, nil
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

func defaultLVMUUIDFunc(ctx context.Context, path string) (string, error) {
	out, err := exec.CommandContext(ctx, "lvs", "--noheadings", "-o", "lv_uuid", path).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// IDsMatch reports whether the devices at src and dest share the same
// identifier. For LVM volumes the logical volume UUID is compared; otherwise
// blkid or GPT serial numbers are used.
func IDsMatch(ctx context.Context, src, dest string) (bool, error) {
	sid, err := GetDeviceID(ctx, src)
	if err != nil {
		return false, err
	}
	did, err := GetDeviceID(ctx, dest)
	if err != nil {
		return false, err
	}
	return sid == did, nil
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
	infos, err := mountinfo.GetMounts(nil)
	if err != nil {
		return false, err
	}
	for _, mi := range infos {
		if mi.Source != real {
			continue
		}
		for _, opt := range strings.Split(mi.Options, ",") {
			if opt == "rw" {
				return true, nil
			}
		}
	}
	return false, nil
}

// SizeBytes returns the total size of the device at path in bytes.
// If ctx has no deadline, a default timeout is applied.
func SizeBytes(ctx context.Context, path string) (uint64, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	dev, err := Detect(ctx, path, true, "", "", "", "", 0, 0, zap.NewNop(), NewRunner())
	if err != nil {
		return 0, err
	}
	defer dev.Close()
	return dev.SizeBytes(), nil
}
