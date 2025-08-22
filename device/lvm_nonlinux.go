//go:build !linux

package device

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"lvmsync_go/internal/lock"
	"lvmsync_go/lvm"
)

// Runner is a stub on non-Linux platforms.
type Runner struct{}

// NewRunner returns a stub runner.
func NewRunner() *Runner { return &Runner{} }

// NewRunnerWithDeps returns a stub runner with custom deps.
func NewRunnerWithDeps(func(context.Context, string) (bool, error), func(context.Context, string) (bool, error), func(context.Context, string) (bool, error), func(context.Context, string) (bool, error), func(string, string) (*lock.Lock, error)) *Runner {
	return &Runner{}
}

// LVMDevice is unsupported on non-Linux platforms.
type LVMDevice struct{}

// OpenLVM returns an error on non-Linux systems.
func (r *Runner) OpenLVM(context.Context, string, *lvm.FDCache, bool, string, *zap.Logger) (*LVMDevice, error) {
	return nil, fmt.Errorf("LVM devices are only supported on Linux")
}

func (d *LVMDevice) Path() string      { return "" }
func (d *LVMDevice) SizeBytes() uint64 { return 0 }
func (d *LVMDevice) BlockSize() uint64 { return 0 }
func (d *LVMDevice) Identity(context.Context) (DeviceIdentity, error) {
	return DeviceIdentity{}, fmt.Errorf("unsupported")
}
func (d *LVMDevice) AppendWAL(r Range) error               { return nil }
func (d *LVMDevice) RecoverWAL(fn func(Range) error) error { return nil }
func (d *LVMDevice) Close() error                          { return nil }
func (d *LVMDevice) Snapshot(context.Context, string) (Device, error) {
	return nil, fmt.Errorf("LVM snapshots are only supported on Linux")
}
func (d *LVMDevice) Cleanup(context.Context) error { return nil }
