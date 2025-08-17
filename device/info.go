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

// DeviceInfoProvider exposes device identification helpers.
type DeviceInfoProvider interface {
	GetUUID(ctx context.Context, path string) (string, error)
	GetLVMUUID(ctx context.Context, path string) (string, error)
	GetDeviceID(ctx context.Context, path string) (string, error)
	IDsMatch(ctx context.Context, src, dest string) (bool, error)
	IsMountedRW(path string) (bool, error)
}

// Info implements DeviceInfoProvider using configurable helpers.
type Info struct {
	uuidFunc    func(context.Context, string) (string, error)
	lvmUUIDFunc func(context.Context, string) (string, error)
	mountFunc   func(string) (bool, error)
}

// NewInfo returns an Info using production dependencies.
func NewInfo() *Info {
	return &Info{
		uuidFunc:    defaultUUIDFunc,
		lvmUUIDFunc: defaultLVMUUIDFunc,
		mountFunc:   defaultMountFunc,
	}
}

// NewInfoWithDeps constructs an Info with custom functions. Nil functions use
// the production implementations.
func NewInfoWithDeps(
	uuid func(context.Context, string) (string, error),
	lvmUUID func(context.Context, string) (string, error),
	mount func(string) (bool, error),
) *Info {
	if uuid == nil {
		uuid = defaultUUIDFunc
	}
	if lvmUUID == nil {
		lvmUUID = defaultLVMUUIDFunc
	}
	if mount == nil {
		mount = defaultMountFunc
	}
	return &Info{uuidFunc: uuid, lvmUUIDFunc: lvmUUID, mountFunc: mount}
}

var defaultInfo = NewInfo()

// SetUUIDFunc allows tests to override the implementation used by the package
// level helpers to lookup a device's UUID or serial. It returns the previous
// function for restoration.
func SetUUIDFunc(f func(context.Context, string) (string, error)) func(context.Context, string) (string, error) {
	prev := defaultInfo.uuidFunc
	defaultInfo.uuidFunc = f
	return prev
}

// SetLVMUUIDFunc allows tests to override the implementation used to lookup an
// LVM logical volume's UUID. It returns the previous function for restoration.
func SetLVMUUIDFunc(f func(context.Context, string) (string, error)) func(context.Context, string) (string, error) {
	prev := defaultInfo.lvmUUIDFunc
	defaultInfo.lvmUUIDFunc = f
	return prev
}

// SetMountFunc allows tests to override the mount status checker. It returns
// the previous function for restoration.
func SetMountFunc(f func(string) (bool, error)) func(string) (bool, error) {
	prev := defaultInfo.mountFunc
	defaultInfo.mountFunc = f
	return prev
}

func (i *Info) GetUUID(ctx context.Context, path string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, uuidTimeout)
		defer cancel()
	}
	return i.uuidFunc(ctx, path)
}

func (i *Info) GetLVMUUID(ctx context.Context, path string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, uuidTimeout)
		defer cancel()
	}
	return i.lvmUUIDFunc(ctx, path)
}

func (i *Info) GetDeviceID(ctx context.Context, path string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, uuidTimeout)
		defer cancel()
	}
	if id, err := i.lvmUUIDFunc(ctx, path); err == nil && id != "" {
		return id, nil
	}
	return i.uuidFunc(ctx, path)
}

func (i *Info) IDsMatch(ctx context.Context, src, dest string) (bool, error) {
	sid, err := i.GetDeviceID(ctx, src)
	if err != nil {
		return false, err
	}
	did, err := i.GetDeviceID(ctx, dest)
	if err != nil {
		return false, err
	}
	return sid == did, nil
}

func (i *Info) IsMountedRW(path string) (bool, error) { return i.mountFunc(path) }

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

// Package-level helpers using the default provider.
func GetUUID(ctx context.Context, path string) (string, error) {
	return defaultInfo.GetUUID(ctx, path)
}

func GetDeviceID(ctx context.Context, path string) (string, error) {
	return defaultInfo.GetDeviceID(ctx, path)
}

func IDsMatch(ctx context.Context, src, dest string) (bool, error) {
	return defaultInfo.IDsMatch(ctx, src, dest)
}

func IsMountedRW(path string) (bool, error) { return defaultInfo.IsMountedRW(path) }

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
