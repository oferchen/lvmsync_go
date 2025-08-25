package lvm

import "context"

// VolumeMetadata represents metadata about a logical volume.
type VolumeMetadata struct {
	VolumeName string
	SizeBytes  uint64
	ChunkSize  uint64
}

// API defines the set of methods required by LVM operations.
type API interface {
	Lock(ctx context.Context, volume, requester string) error
	Unlock(ctx context.Context, volume, requester string) error
	GetMetadata(ctx context.Context, volume string) (VolumeMetadata, error)
	SendMetadata(ctx context.Context, md VolumeMetadata) error
	StartTransferSession(ctx context.Context, volume, requester string) error
	FinalizeSync(ctx context.Context, volume, requester string) error
	GetStatus(ctx context.Context, volume, requester string) (string, error)
	VolumeExists(ctx context.Context, volume string) (bool, error)
	AutoExtendEnabled(ctx context.Context, volume string) (bool, error)
	DiscardEnabled(ctx context.Context, volume string) (bool, error)
	IsMounted(ctx context.Context, volume string) (bool, error)
}
