package device

import (
	"context"
	"os/exec"
	"time"

	"go.uber.org/zap"

	"lvmsync_go/internal/lock"
	"lvmsync_go/internal/privilege"
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
	openLVMOverride   func(context.Context, string, *lvm.FDCache, bool, bool, string, *zap.Logger) (*LVMDevice, error)
	openRawOverride   func(context.Context, string, bool, bool, string, []string, string, []string, time.Duration, time.Duration, privilege.Escalator, *zap.Logger) (*RawDevice, error)
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
		isMountedRW:       defaultIsMountedRW,
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

func defaultIsMountedRW(ctx context.Context, path string) (bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, mountTimeout)
		defer cancel()
	}
	return defaultMountFunc(ctx, path)
}

// OpenRaw wraps the package-level OpenRaw allowing tests to override the implementation.
func (r *Runner) OpenRaw(
	ctx context.Context,
	path string,
	readonly bool,
	offline bool,
	freezePath string,
	freezeArgs []string,
	thawPath string,
	thawArgs []string,
	freezeTimeout time.Duration,
	thawTimeout time.Duration,
	esc privilege.Escalator,
	logger *zap.Logger,
) (*RawDevice, error) {
	if r.openRawOverride != nil {
		return r.openRawOverride(ctx, path, readonly, offline, freezePath, freezeArgs, thawPath, thawArgs, freezeTimeout, thawTimeout, esc, logger)
	}
	return OpenRaw(ctx, path, readonly, offline, freezePath, freezeArgs, thawPath, thawArgs, freezeTimeout, thawTimeout, esc, logger, r)
}
