package lvm

import (
	"context"
	"errors"

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

// lvmAPI defines the subset of methods used by the agent. It allows tests to
// inject a mock implementation while the real implementation can satisfy the
// interface implicitly.
type lvmAPI interface {
	Lock(context.Context, string, string) error
	Unlock(context.Context, string, string) error
	GetMetadata(context.Context, string) (VolumeMetadata, error)
	SendMetadata(context.Context, VolumeMetadata) error
	StartTransferSession(context.Context, string, string) error
	FinalizeSync(context.Context, string, string) error
	GetStatus(context.Context, string, string) (string, error)
}

// sudoAgent ensures root privileges via sudo before invoking LVM commands.
type sudoAgent struct {
	sudoPath   string
	lvm        lvmAPI
	ensureRoot func() error
}

// NewSudoAgent returns an Agent implementation that escalates privileges using sudo.
// An optional ensureRoot function can be provided (primarily for tests). When nil,
// privesc.EnsureRoot is used with the configured sudo path.
func NewSudoAgent(sudoPath string, l lvmlib.API, ensureRoot func() error) Agent {
	api, _ := l.(lvmAPI)
	if ensureRoot == nil {
		ensureRoot = func() error {
			cmd := sudoPath
			if cmd == "" {
				cmd = "sudo -n"
			}
			return privesc.EnsureRoot(cmd)
		}
	}
	return &sudoAgent{sudoPath: sudoPath, lvm: api, ensureRoot: ensureRoot}
}

func (s *sudoAgent) Lock(ctx context.Context, volume, requester string) error {
	if err := s.ensureRoot(); err != nil {
		return err
	}
	if s.lvm == nil {
		return errors.New("lvm api not provided")
	}
	return s.lvm.Lock(ctx, volume, requester)
}

func (s *sudoAgent) Unlock(ctx context.Context, volume, requester string) error {
	if err := s.ensureRoot(); err != nil {
		return err
	}
	if s.lvm == nil {
		return errors.New("lvm api not provided")
	}
	return s.lvm.Unlock(ctx, volume, requester)
}

func (s *sudoAgent) GetMetadata(ctx context.Context, volume string) (VolumeMetadata, error) {
	if err := s.ensureRoot(); err != nil {
		return VolumeMetadata{}, err
	}
	if s.lvm == nil {
		return VolumeMetadata{}, errors.New("lvm api not provided")
	}
	return s.lvm.GetMetadata(ctx, volume)
}

func (s *sudoAgent) SendMetadata(ctx context.Context, md VolumeMetadata) error {
	if err := s.ensureRoot(); err != nil {
		return err
	}
	if s.lvm == nil {
		return errors.New("lvm api not provided")
	}
	return s.lvm.SendMetadata(ctx, md)
}

func (s *sudoAgent) StartTransferSession(ctx context.Context, volume, requester string) error {
	if err := s.ensureRoot(); err != nil {
		return err
	}
	if s.lvm == nil {
		return errors.New("lvm api not provided")
	}
	return s.lvm.StartTransferSession(ctx, volume, requester)
}

func (s *sudoAgent) FinalizeSync(ctx context.Context, volume, requester string) error {
	if err := s.ensureRoot(); err != nil {
		return err
	}
	if s.lvm == nil {
		return errors.New("lvm api not provided")
	}
	return s.lvm.FinalizeSync(ctx, volume, requester)
}

func (s *sudoAgent) GetStatus(ctx context.Context, volume, requester string) (string, error) {
	if err := s.ensureRoot(); err != nil {
		return "", err
	}
	if s.lvm == nil {
		return "", errors.New("lvm api not provided")
	}
	return s.lvm.GetStatus(ctx, volume, requester)
}
