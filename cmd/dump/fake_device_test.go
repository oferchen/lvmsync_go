package dump

import (
	"context"

	"lvmsync_go/device"
)

type fakeDevice struct{ path string }

func (f *fakeDevice) Path() string                                            { return f.path }
func (f *fakeDevice) SizeBytes() uint64                                       { return 0 }
func (f *fakeDevice) BlockSize() uint64                                       { return 0 }
func (f *fakeDevice) Snapshot(context.Context, string) (device.Device, error) { return f, nil }
func (f *fakeDevice) Cleanup(context.Context) error                           { return nil }
func (f *fakeDevice) Close() error                                            { return nil }
func (f *fakeDevice) Identity(context.Context) (device.DeviceIdentity, error) {
	return device.DeviceIdentity{}, nil
}
func (f *fakeDevice) AppendWAL(r device.Range) error               { return nil }
func (f *fakeDevice) RecoverWAL(fn func(device.Range) error) error { return nil }
