package device

import (
	"bytes"

	wal "github.com/oferchen/lvmsync_go/internal/wal"
)

// DeviceIdentity describes metadata uniquely identifying a block device.
// The identity tuple is (size_bytes, kernel_uuid, gpt_uuid, mbr_signature,
// fs_uuid, partition_hash, major, minor, manifest_epoch).
type DeviceIdentity struct { //nolint:revive // name stutters with package but kept for clarity
	SizeBytes     uint64
	KernelUUID    string
	GPTUUID       string
	MBRSignature  string
	FSUUID        string
	PartitionHash [32]byte
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
		bytes.Equal(a.PartitionHash[:], b.PartitionHash[:]) &&
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
		bytes.Equal(a.PartitionHash[:], b.PartitionHash[:]) &&
		a.Major == b.Major &&
		a.Minor == b.Minor &&
		a.ManifestEpoch == b.ManifestEpoch
}

// Range represents a byte range on a device.
type Range = wal.Range
