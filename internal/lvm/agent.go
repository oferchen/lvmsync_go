package lvm

import (
	"context"

	"lvmsync_go/internal/privesc"
	lvmlib "lvmsync_go/lvm"
)

// VolumeMetadata represents metadata about a logical volume.
type VolumeMetadata struct {
	VolumeName string
	SizeBytes  uint64
	ChunkSize  uint64
}

// Agent defines methods for performing privileged LVM operations.
type Agent interface {
	Lock(_ context.Context, volume, requester string) error
	Unlock(_ context.Context, volume, requester string) error
	GetMetadata(_ context.Context, volume string) (VolumeMetadata, error)
	SendMetadata(_ context.Context, md VolumeMetadata) error
	StartTransferSession(_ context.Context, volume, requester string) error
	FinalizeSync(_ context.Context, volume, requester string) error
	GetStatus(_ context.Context, volume, requester string) (string, error)
}

// sudoAgent ensures root privileges via sudo before invoking LVM commands.
type sudoAgent struct {
	sudoPath string
	lvm      lvmlib.API
}

// NewSudoAgent returns an Agent implementation that escalates privileges using sudo.
func NewSudoAgent(sudoPath string, l lvmlib.API) Agent {
	return &sudoAgent{sudoPath: sudoPath, lvm: l}
}

func (s *sudoAgent) ensureRoot() error {
	cmd := s.sudoPath
	if cmd == "" {
		cmd = "sudo -n"
	}
	return privesc.EnsureRoot(cmd)
}

func (s *sudoAgent) Lock(_ context.Context, _, _ string) error {
	if err := s.ensureRoot(); err != nil {
		return err
	}
	// TODO: invoke lvm locking once available
	return nil
}

func (s *sudoAgent) Unlock(_ context.Context, _, _ string) error {
	if err := s.ensureRoot(); err != nil {
		return err
	}
	// TODO: invoke lvm unlock once available
	return nil
}

func (s *sudoAgent) GetMetadata(_ context.Context, _ string) (VolumeMetadata, error) {
	if err := s.ensureRoot(); err != nil {
		return VolumeMetadata{}, err
	}
	// TODO: fetch metadata using lvm
	return VolumeMetadata{}, nil
}

func (s *sudoAgent) SendMetadata(_ context.Context, _ VolumeMetadata) error {
	if err := s.ensureRoot(); err != nil {
		return err
	}
	// TODO: send metadata using lvm
	return nil
}

func (s *sudoAgent) StartTransferSession(_ context.Context, volume, requester string) error {
	if err := s.ensureRoot(); err != nil {
		return err
	}
	// TODO: start transfer session via lvm
	return nil
}

func (s *sudoAgent) FinalizeSync(_ context.Context, volume, requester string) error {
	if err := s.ensureRoot(); err != nil {
		return err
	}
	// TODO: finalize sync via lvm
	return nil
}

func (s *sudoAgent) GetStatus(_ context.Context, volume, requester string) (string, error) {
	if err := s.ensureRoot(); err != nil {
		return "", err
	}
	// TODO: query status via lvm
	return "", nil
}
