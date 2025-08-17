package device

import (
	"context"
	"os/exec"
)

type cmdFunc func(context.Context, string, ...string) *exec.Cmd

func (f cmdFunc) CommandContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	return f(ctx, name, args...)
}
