package device

// DeviceIdentity describes metadata uniquely identifying a block device.
// The identity tuple is (size_bytes, kernel_uuid, gpt_uuid, mbr_signature, fs_uuid, major, minor, manifest_epoch).
type DeviceIdentity struct {
	SizeBytes     uint64
	KernelUUID    string
	GPTUUID       string
	MBRSignature  string
	FSUUID        string
	Major         uint32
	Minor         uint32
	ManifestEpoch uint64
}

// SameIdentity reports whether two DeviceIdentity values describe the same
// device based on stable fields.
//
// Kernel-assigned Major and Minor numbers can change across reboots or when
// devices are reattached, so they are intentionally ignored.
func SameIdentity(a, b DeviceIdentity) bool {
	return a.SizeBytes == b.SizeBytes &&
		a.KernelUUID == b.KernelUUID &&
		a.GPTUUID == b.GPTUUID &&
		a.MBRSignature == b.MBRSignature &&
		a.FSUUID == b.FSUUID &&
		a.ManifestEpoch == b.ManifestEpoch
}

// SameIdentityStrict reports whether two DeviceIdentity values describe the
// same device including kernel-assigned Major and Minor numbers.
func SameIdentityStrict(a, b DeviceIdentity) bool {
	return a.SizeBytes == b.SizeBytes &&
		a.KernelUUID == b.KernelUUID &&
		a.GPTUUID == b.GPTUUID &&
		a.MBRSignature == b.MBRSignature &&
		a.FSUUID == b.FSUUID &&
		a.Major == b.Major &&
		a.Minor == b.Minor &&
		a.ManifestEpoch == b.ManifestEpoch
}

// Range represents a byte range on a device.
type Range struct {
	Start uint64
	End   uint64
}
