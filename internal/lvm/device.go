package lvm

import (
	"context"
	"fmt"
	"path/filepath"
)

// Checker validates LVM volumes before use and releases locks after writes.
// Requester identifies the lock holder.
type Checker struct {
	Agent     Agent
	Requester string
	DevRoot   string
}

// PreOpen ensures the logical volume exists and is ready for writes.
// It locks the volume and returns the device path.
func (c Checker) PreOpen(ctx context.Context, vg, lv string) (string, error) {
	vol := filepath.Join(vg, lv)
	ok, err := c.Agent.VolumeExists(ctx, vol)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("volume %s not found", vol)
	}
	if ok, err := c.Agent.AutoExtendEnabled(ctx, vol); err != nil {
		return "", err
	} else if !ok {
		return "", fmt.Errorf("auto-extend disabled")
	}
	if ok, err := c.Agent.DiscardEnabled(ctx, vol); err != nil {
		return "", err
	} else if !ok {
		return "", fmt.Errorf("discard disabled")
	}
	if mounted, err := c.Agent.IsMounted(ctx, vol); err != nil {
		return "", err
	} else if mounted {
		return "", fmt.Errorf("volume %s mounted", vol)
	}
	if err := c.Agent.Lock(ctx, vol, c.Requester); err != nil {
		return "", err
	}
	root := c.DevRoot
	if root == "" {
		root = "/dev"
	}
	return filepath.Join(root, vg, lv), nil
}

// PostCommit fsyncs the device and releases the lock.
func (c Checker) PostCommit(ctx context.Context, vg, lv string, f interface{ Sync() error }) error {
	if err := f.Sync(); err != nil {
		return err
	}
	vol := filepath.Join(vg, lv)
	return c.Agent.Unlock(ctx, vol, c.Requester)
}
