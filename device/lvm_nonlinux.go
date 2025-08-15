//go:build !linux

package device

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"lvmsync_go/lvm"
)

// LVMDevice is unsupported on non-Linux platforms.
type LVMDevice struct{}

// OpenLVM returns an error on non-Linux systems.
func OpenLVM(string, *lvm.FDCache, *zap.Logger) (*LVMDevice, error) {
	return nil, fmt.Errorf("LVM devices are only supported on Linux")
}

func (d *LVMDevice) Path() string      { return "" }
func (d *LVMDevice) SizeBytes() uint64 { return 0 }
func (d *LVMDevice) BlockSize() uint64 { return 0 }
func (d *LVMDevice) Close() error      { return nil }
func (d *LVMDevice) Snapshot(context.Context, string) (Device, error) {
	return nil, fmt.Errorf("LVM snapshots are only supported on Linux")
}
func (d *LVMDevice) Cleanup(context.Context) error { return nil }
