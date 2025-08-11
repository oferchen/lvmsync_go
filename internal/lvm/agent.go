// Package lvm exposes a minimal privileged agent wrapper used by the gRPC
// server. It delegates to an injected Escalator to ensure required
// privileges before invoking the underlying LVM API.
package lvm

import (
	"context"
	"errors"

	"lvmsync_go/internal/privilege"
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
	Lock(ctx context.Context, volume, requester string) error
	Unlock(ctx context.Context, volume, requester string) error
	GetMetadata(ctx context.Context, volume string) (VolumeMetadata, error)
	SendMetadata(ctx context.Context, md VolumeMetadata) error
	StartTransferSession(ctx context.Context, volume, requester string) error
	FinalizeSync(ctx context.Context, volume, requester string) error
	GetStatus(ctx context.Context, volume, requester string) (string, error)
}

// lvmAPI describes the subset of the LVM library used by the agent.
type lvmAPI interface {
	Lock(context.Context, string, string) error
	Unlock(context.Context, string, string) error
	GetMetadata(context.Context, string) (VolumeMetadata, error)
	SendMetadata(context.Context, VolumeMetadata) error
	StartTransferSession(context.Context, string, string) error
	FinalizeSync(context.Context, string, string) error
	GetStatus(context.Context, string, string) (string, error)
}

// agent ensures privileges via an Escalator before delegating to the LVM API.
type agent struct {
	esc privilege.Escalator
	lvm lvmAPI
}

// NewAgent creates an Agent using the provided LVM API and Escalator. When esc
// is nil, the default Escalator is used.
func NewAgent(l lvmlib.API, esc privilege.Escalator) Agent {
	api, _ := l.(lvmAPI)
	if esc == nil {
		esc = privilege.New()
	}
	return &agent{esc: esc, lvm: api}
}

func (a *agent) Lock(ctx context.Context, volume, requester string) error {
	if err := a.esc.Ensure(); err != nil {
		return err
	}
	if a.lvm == nil {
		return errors.New("lvm api not provided")
	}
	return a.lvm.Lock(ctx, volume, requester)
}

func (a *agent) Unlock(ctx context.Context, volume, requester string) error {
	if err := a.esc.Ensure(); err != nil {
		return err
	}
	if a.lvm == nil {
		return errors.New("lvm api not provided")
	}
	return a.lvm.Unlock(ctx, volume, requester)
}

func (a *agent) GetMetadata(ctx context.Context, volume string) (VolumeMetadata, error) {
	if err := a.esc.Ensure(); err != nil {
		return VolumeMetadata{}, err
	}
	if a.lvm == nil {
		return VolumeMetadata{}, errors.New("lvm api not provided")
	}
	return a.lvm.GetMetadata(ctx, volume)
}

func (a *agent) SendMetadata(ctx context.Context, md VolumeMetadata) error {
	if err := a.esc.Ensure(); err != nil {
		return err
	}
	if a.lvm == nil {
		return errors.New("lvm api not provided")
	}
	return a.lvm.SendMetadata(ctx, md)
}

func (a *agent) StartTransferSession(ctx context.Context, volume, requester string) error {
	if err := a.esc.Ensure(); err != nil {
		return err
	}
	if a.lvm == nil {
		return errors.New("lvm api not provided")
	}
	return a.lvm.StartTransferSession(ctx, volume, requester)
}

func (a *agent) FinalizeSync(ctx context.Context, volume, requester string) error {
	if err := a.esc.Ensure(); err != nil {
		return err
	}
	if a.lvm == nil {
		return errors.New("lvm api not provided")
	}
	return a.lvm.FinalizeSync(ctx, volume, requester)
}

func (a *agent) GetStatus(ctx context.Context, volume, requester string) (string, error) {
	if err := a.esc.Ensure(); err != nil {
		return "", err
	}
	if a.lvm == nil {
		return "", errors.New("lvm api not provided")
	}
	return a.lvm.GetStatus(ctx, volume, requester)
}
