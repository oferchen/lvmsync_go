//go:build !linux

package device

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
)

// RawDevice is unsupported on non-Linux platforms.
type RawDevice struct{}

// OpenRaw returns an error on non-Linux systems.
func OpenRaw(context.Context, string, bool, string, []string, string, []string, time.Duration, time.Duration, *zap.Logger) (*RawDevice, error) {
	return nil, fmt.Errorf("raw devices are only supported on Linux")
}

func (d *RawDevice) Path() string      { return "" }
func (d *RawDevice) SizeBytes() uint64 { return 0 }
func (d *RawDevice) BlockSize() uint64 { return 0 }
func (d *RawDevice) Snapshot(context.Context, string) (Device, error) {
	return nil, fmt.Errorf("raw device snapshots are only supported on Linux")
}
func (d *RawDevice) Cleanup(context.Context) error { return nil }
func (d *RawDevice) Close() error                  { return nil }
