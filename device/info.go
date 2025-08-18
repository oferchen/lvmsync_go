package device

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/moby/sys/mountinfo"
	"go.uber.org/zap"

	"lvmsync_go/common"
	"lvmsync_go/hash"
	"lvmsync_go/internal/privilege"
)

const (
	uuidTimeout  = 5 * time.Second
	mountTimeout = 5 * time.Second
)

// DeviceInfoProvider exposes device identification helpers.
type DeviceInfoProvider interface {
	GetUUID(ctx context.Context, path string) (string, error)
	GetLVMUUID(ctx context.Context, path string) (string, error)
	GetDeviceID(ctx context.Context, path string) (string, error)
	IDsMatch(ctx context.Context, src, dest string) (bool, error)
	IsMountedRW(ctx context.Context, path string) (bool, error)
	SizeBytes(ctx context.Context, path string) (uint64, error)
	FirstBlockDigest(ctx context.Context, path string, size uint64) ([32]byte, error)
}

// Info implements DeviceInfoProvider using configurable helpers.
type Info struct {
	uuidFunc    func(context.Context, string) (string, error)
	lvmUUIDFunc func(context.Context, string) (string, error)
	mountFunc   func(context.Context, string) (bool, error)
	firstDigest func(context.Context, string, uint64) ([32]byte, error)
	detectFunc  func(context.Context, string, bool, string, string, string, string, time.Duration, time.Duration, privilege.Escalator, *zap.Logger, *Runner) (Device, error)
}

// NewInfo returns an Info using production dependencies.
func NewInfo() *Info {
	return &Info{
		uuidFunc:    defaultUUIDFunc,
		lvmUUIDFunc: defaultLVMUUIDFunc,
		mountFunc:   defaultMountFunc,
		firstDigest: defaultFirstBlockDigest,
		detectFunc:  Detect,
	}
}

// NewInfoWithDeps constructs an Info with custom functions. Nil functions use
// the production implementations.
func NewInfoWithDeps(
	uuid func(context.Context, string) (string, error),
	lvmUUID func(context.Context, string) (string, error),
	mount func(context.Context, string) (bool, error),
	digest func(context.Context, string, uint64) ([32]byte, error),
	detect func(context.Context, string, bool, string, string, string, string, time.Duration, time.Duration, privilege.Escalator, *zap.Logger, *Runner) (Device, error),
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
	if digest == nil {
		digest = defaultFirstBlockDigest
	}
	if detect == nil {
		detect = Detect
	}
	return &Info{uuidFunc: uuid, lvmUUIDFunc: lvmUUID, mountFunc: mount, firstDigest: digest, detectFunc: detect}
}

// SetMountFunc allows tests to override the mount status checker. It returns
// the previous function for restoration.
func (i *Info) SetMountFunc(f func(context.Context, string) (bool, error)) func(context.Context, string) (bool, error) {
	prev := i.mountFunc
	i.mountFunc = f
	return prev
}

// SetDetectFunc allows tests to override the device detector. It returns the previous function for restoration.
func (i *Info) SetDetectFunc(
	f func(context.Context, string, bool, string, string, string, string, time.Duration, time.Duration, privilege.Escalator, *zap.Logger, *Runner) (Device, error),
) func(context.Context, string, bool, string, string, string, string, time.Duration, time.Duration, privilege.Escalator, *zap.Logger, *Runner) (Device, error) {
	prev := i.detectFunc
	i.detectFunc = f
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

func (i *Info) IsMountedRW(ctx context.Context, path string) (bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, mountTimeout)
		defer cancel()
	}
	return i.mountFunc(ctx, path)
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

// SizeBytes returns the total size of the device at path in bytes.
// If ctx has no deadline, a default timeout is applied.
func (i *Info) SizeBytes(ctx context.Context, path string) (size uint64, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	dev, err := i.detectFunc(ctx, path, true, "", "", "", "", 0, 0, privilege.New(ctx), zap.NewNop(), NewRunner())
	if err != nil {
		return 0, err
	}
	defer func() {
		if closeErr := dev.Close(); closeErr != nil {
			err = fmt.Errorf("close block device: %w", closeErr)
		}
	}()
	size = dev.SizeBytes()
	return size, err
}

// FirstBlockDigest returns the BLAKE3 digest of the first size bytes of the device at path.
// If ctx has no deadline, a default timeout is applied.
func (i *Info) FirstBlockDigest(ctx context.Context, path string, size uint64) ([32]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, uuidTimeout)
		defer cancel()
	}
	return i.firstDigest(ctx, path, size)
}

func defaultMountFunc(ctx context.Context, path string) (bool, error) {
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return false, err
	}
	type result struct {
		infos []*mountinfo.Info
		err   error
	}
	ch := make(chan result, 1)
	go func() {
		infos, err := mountinfo.GetMounts(nil)
		ch <- result{infos: infos, err: err}
	}()
	var infos []*mountinfo.Info
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	case r := <-ch:
		if r.err != nil {
			return false, r.err
		}
		infos = r.infos
	}
	for _, mi := range infos {
		if mi.Source != real && mi.Mountpoint != real && mi.Root != real {
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

func defaultFirstBlockDigest(ctx context.Context, path string, size uint64) ([32]byte, error) {
	var out [32]byte
	if size == 0 {
		return out, fmt.Errorf("size must be greater than zero")
	}
	f, err := common.OpenWithContext(ctx, path)
	if err != nil {
		return out, err
	}
	defer f.Close()
	buf := make([]byte, size)
	n, err := io.ReadFull(f, buf)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return out, err
	}
	return hash.SumBLAKE3(buf[:n]), nil
}
