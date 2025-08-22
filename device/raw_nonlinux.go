//go:build !linux

package device

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/oferchen/lvmsync_go/internal/privilege"
)

// RawDevice is unsupported on non-Linux platforms.
type RawDevice struct{}

// OpenRaw returns an error on non-Linux systems.
func OpenRaw(context.Context, string, bool, string, []string, string, []string, time.Duration, time.Duration, privilege.Escalator, *zap.Logger, *Runner) (*RawDevice, error) {
	return nil, fmt.Errorf("raw devices are only supported on Linux")
}

func (d *RawDevice) Path() string      { return "" }
func (d *RawDevice) SizeBytes() uint64 { return 0 }
func (d *RawDevice) BlockSize() uint64 { return 0 }
func (d *RawDevice) Identity(context.Context) (DeviceIdentity, error) {
	return DeviceIdentity{}, fmt.Errorf("unsupported")
}
func (d *RawDevice) AppendWAL(r Range) error               { return nil }
func (d *RawDevice) RecoverWAL(fn func(Range) error) error { return nil }
func (d *RawDevice) Snapshot(context.Context, string) (Device, error) {
	return nil, fmt.Errorf("raw device snapshots are only supported on Linux")
}
func (d *RawDevice) Cleanup(context.Context) error { return nil }
func (d *RawDevice) Close() error                  { return nil }
