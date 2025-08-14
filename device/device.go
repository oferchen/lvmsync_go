package device

import "context"

// Device describes a block device capable of reporting size information.
type Device interface {
	// Path returns the device path.
	Path() string
	// SizeBytes returns the total size of the device in bytes.
	SizeBytes() uint64
	// BlockSize returns the logical block size of the device in bytes.
	BlockSize() uint64
	// Snapshot prepares the device for a consistent read, returning a
	// handle that should be closed after use.
	Snapshot(ctx context.Context) (Device, error)
	// Cleanup releases any snapshot resources held by the device.
	Cleanup(ctx context.Context) error
	// Close releases any resources associated with the device.
	Close() error
}
