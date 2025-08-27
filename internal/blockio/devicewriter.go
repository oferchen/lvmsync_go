package blockio

import (
	"context"
	"path/filepath"

	"github.com/oferchen/lvmsync_go/internal/lvm"
)

// DeviceWriter opens LVM logical volumes for writing.
// When strict is true, misaligned writes or failed O_DIRECT opens error.
type DeviceWriter struct {
	Checker lvm.Checker
	Strict  bool
}

// Open prepares the logical volume and returns a file with a close callback.
func (d DeviceWriter) Open(ctx context.Context, vg, lv string, direct bool) (*File, func() error, error) {
	path, err := d.Checker.PreOpen(ctx, vg, lv)
	if err != nil {
		return nil, nil, err
	}
	f, err := Open(path, direct, d.Strict)
	if err != nil {
		_ = d.Checker.Agent.Unlock(ctx, filepath.Join(vg, lv), d.Checker.Requester)
		return nil, nil, err
	}
	closeFn := func() error {
		if err := d.Checker.PostCommit(ctx, vg, lv, f); err != nil {
			_ = f.Close()
			return err
		}
		return f.Close()
	}
	return f, closeFn, nil
}
