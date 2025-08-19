package device

// DeviceIdentity describes metadata uniquely identifying a block device.
type DeviceIdentity struct {
	SizeBytes  uint64
	KernelUUID string
	GPTUUID    string
	FSUUID     string
}

// Range represents a byte range on a device.
type Range struct {
	Start uint64
	End   uint64
}
