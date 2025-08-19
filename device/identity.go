package device

// DeviceIdentity describes metadata uniquely identifying a block device.
// The identity tuple is (size_bytes, kernel_uuid, gpt_uuid, fs_uuid, major, minor).
type DeviceIdentity struct {
	SizeBytes  uint64
	KernelUUID string
	GPTUUID    string
	FSUUID     string
	Major      uint32
	Minor      uint32
}

// Range represents a byte range on a device.
type Range struct {
	Start uint64
	End   uint64
}
