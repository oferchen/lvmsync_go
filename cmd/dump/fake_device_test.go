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
