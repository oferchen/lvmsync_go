// Package lvm provides a privileged agent wrapper around LVM operations.
package lvm

import (
	"context"
	"errors"

	"go.uber.org/zap"

	"lvmsync_go/internal/privilege"
	lvmlib "lvmsync_go/lvm"
)

// Agent defines methods for performing privileged LVM operations.
type Agent interface {
	Lock(ctx context.Context, volume, requester string) error
	Unlock(ctx context.Context, volume, requester string) error
	GetMetadata(ctx context.Context, volume string) (lvmlib.VolumeMetadata, error)
	SendMetadata(ctx context.Context, md lvmlib.VolumeMetadata) error
	StartTransferSession(ctx context.Context, volume, requester string) error
	FinalizeSync(ctx context.Context, volume, requester string) error
	GetStatus(ctx context.Context, volume, requester string) (string, error)
	VolumeExists(ctx context.Context, volume string) (bool, error)
	AutoExtendEnabled(ctx context.Context, volume string) (bool, error)
	DiscardEnabled(ctx context.Context, volume string) (bool, error)
	IsMounted(ctx context.Context, volume string) (bool, error)
}

// agent ensures privileges via an Escalator before delegating to the LVM API.
type agent struct {
	esc privilege.Escalator
	lvm lvmlib.API
}

// NewAgent constructs an Agent that delegates LVM operations to the provided
// API and ensures privileges using the given Escalator. When esc is nil,
// privilege.New(context.Background(), logger) supplies a default implementation.
func NewAgent(lvm lvmlib.API, esc privilege.Escalator, logger *zap.Logger) Agent {
	if logger == nil {
		logger = zap.NewNop()
	}
	if esc == nil {
		esc = privilege.New(context.Background(), logger)
	}
	return &agent{esc: esc, lvm: lvm}
}

// NewSudoAgent wraps the public LVM API with a privilege-enforcing Agent. The
// sudoPath and ensureRoot parameters are retained for compatibility but are
// currently unused.
func NewSudoAgent(sudoPath string, l lvmlib.API, ensureRoot func() error, logger *zap.Logger) Agent { //nolint:revive // sudoPath kept for future use
	_ = sudoPath
	_ = ensureRoot
	return NewAgent(l, nil, logger)
}

func (a *agent) Lock(ctx context.Context, volume, requester string) error {
	if err := a.esc.Ensure(ctx); err != nil {
		return err
	}
	if a.lvm == nil {
		return errors.New("lvm api not provided")
	}
	return a.lvm.Lock(ctx, volume, requester)
}

func (a *agent) Unlock(ctx context.Context, volume, requester string) error {
	if err := a.esc.Ensure(ctx); err != nil {
		return err
	}
	if a.lvm == nil {
		return errors.New("lvm api not provided")
	}
	return a.lvm.Unlock(ctx, volume, requester)
}

func (a *agent) GetMetadata(ctx context.Context, volume string) (lvmlib.VolumeMetadata, error) {
	if err := a.esc.Ensure(ctx); err != nil {
		return lvmlib.VolumeMetadata{}, err
	}
	if a.lvm == nil {
		return lvmlib.VolumeMetadata{}, errors.New("lvm api not provided")
	}
	return a.lvm.GetMetadata(ctx, volume)
}

func (a *agent) SendMetadata(ctx context.Context, md lvmlib.VolumeMetadata) error {
	if err := a.esc.Ensure(ctx); err != nil {
		return err
	}
	if a.lvm == nil {
		return errors.New("lvm api not provided")
	}
	return a.lvm.SendMetadata(ctx, md)
}

func (a *agent) StartTransferSession(ctx context.Context, volume, requester string) error {
	if err := a.esc.Ensure(ctx); err != nil {
		return err
	}
	if a.lvm == nil {
		return errors.New("lvm api not provided")
	}
	return a.lvm.StartTransferSession(ctx, volume, requester)
}

func (a *agent) FinalizeSync(ctx context.Context, volume, requester string) error {
	if err := a.esc.Ensure(ctx); err != nil {
		return err
	}
	if a.lvm == nil {
		return errors.New("lvm api not provided")
	}
	return a.lvm.FinalizeSync(ctx, volume, requester)
}

func (a *agent) GetStatus(ctx context.Context, volume, requester string) (string, error) {
	if err := a.esc.Ensure(ctx); err != nil {
		return "", err
	}
	if a.lvm == nil {
		return "", errors.New("lvm api not provided")
	}
	return a.lvm.GetStatus(ctx, volume, requester)
}

func (a *agent) VolumeExists(ctx context.Context, volume string) (bool, error) {
	if err := a.esc.Ensure(ctx); err != nil {
		return false, err
	}
	if a.lvm == nil {
		return false, errors.New("lvm api not provided")
	}
	return a.lvm.VolumeExists(ctx, volume)
}

func (a *agent) AutoExtendEnabled(ctx context.Context, volume string) (bool, error) {
	if err := a.esc.Ensure(ctx); err != nil {
		return false, err
	}
	if a.lvm == nil {
		return false, errors.New("lvm api not provided")
	}
	return a.lvm.AutoExtendEnabled(ctx, volume)
}

func (a *agent) DiscardEnabled(ctx context.Context, volume string) (bool, error) {
	if err := a.esc.Ensure(ctx); err != nil {
		return false, err
	}
	if a.lvm == nil {
		return false, errors.New("lvm api not provided")
	}
	return a.lvm.DiscardEnabled(ctx, volume)
}

func (a *agent) IsMounted(ctx context.Context, volume string) (bool, error) {
	if err := a.esc.Ensure(ctx); err != nil {
		return false, err
	}
	if a.lvm == nil {
		return false, errors.New("lvm api not provided")
	}
	return a.lvm.IsMounted(ctx, volume)
}
