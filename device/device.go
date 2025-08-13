package device

// Device describes a block device capable of reporting size information.
type Device interface {
	// Path returns the device path.
	Path() string
	// SizeBytes returns the total size of the device in bytes.
	SizeBytes() uint64
	// BlockSize returns the logical block size of the device in bytes.
	BlockSize() uint64
	// Close releases any resources associated with the device.
	Close() error
}
