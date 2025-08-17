package device

import (
	"context"
	"os/exec"

	"go.uber.org/zap"

	"lvmsync_go/internal/lock"
	"lvmsync_go/lvm"
)

// Commander abstracts exec.CommandContext for testability.
type Commander interface {
	CommandContext(ctx context.Context, name string, args ...string) *exec.Cmd
}

type commanderFunc func(context.Context, string, ...string) *exec.Cmd

func (f commanderFunc) CommandContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	return f(ctx, name, args...)
}

// Runner coordinates interactions with external helpers.
type Runner struct {
	command           Commander
	volumeExists      func(context.Context, string) (bool, error)
	autoExtendEnabled func(context.Context, string) (bool, error)
	discardEnabled    func(context.Context, string) (bool, error)
	isMountedRW       func(context.Context, string) (bool, error)
	lockAcquire       func(string, string) (*lock.Lock, error)
	openLVMOverride   func(string, *lvm.FDCache, string, *zap.Logger) (*LVMDevice, error)
}

// NewDeviceRunner returns a Runner using production dependencies and the provided Commander.
// If cmd is nil, exec.CommandContext is used.
func NewDeviceRunner(cmd Commander) *Runner {
	if cmd == nil {
		cmd = commanderFunc(exec.CommandContext)
	}
	return &Runner{
		command:           cmd,
		volumeExists:      lvm.VolumeExists,
		autoExtendEnabled: lvm.AutoExtendEnabled,
		discardEnabled:    lvm.DiscardEnabled,
		isMountedRW:       defaultInfo.IsMountedRW,
		lockAcquire:       lock.Acquire,
	}
}

// NewRunner is retained for compatibility; it uses exec.CommandContext.
func NewRunner() *Runner { return NewDeviceRunner(nil) }

// NewRunnerWithDeps constructs a Runner with custom dependencies.
// If cmd is nil, exec.CommandContext is used.
func NewRunnerWithDeps(
	volumeExists func(context.Context, string) (bool, error),
	autoExtendEnabled func(context.Context, string) (bool, error),
	discardEnabled func(context.Context, string) (bool, error),
	isMountedRW func(context.Context, string) (bool, error),
	lockAcquire func(string, string) (*lock.Lock, error),
	cmd Commander,
) *Runner {
	if cmd == nil {
		cmd = commanderFunc(exec.CommandContext)
	}
	return &Runner{
		command:           cmd,
		volumeExists:      volumeExists,
		autoExtendEnabled: autoExtendEnabled,
		discardEnabled:    discardEnabled,
		isMountedRW:       isMountedRW,
		lockAcquire:       lockAcquire,
	}
}
