package privilege

import (
	"context"
	"os/exec"
)

// Commander abstracts exec.CommandContext for dependency injection.
type Commander interface {
	CommandContext(ctx context.Context, name string, args ...string) *exec.Cmd
}

type commanderFunc func(context.Context, string, ...string) *exec.Cmd

func (f commanderFunc) CommandContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	return f(ctx, name, args...)
}

// Runner provides external dependencies for privilege escalation.
type Runner struct {
	Cmd      Commander
	LookPath func(string) (string, error)
}

// New returns an Escalator with production dependencies.
func New() Escalator { return NewWithRunner(nil) }

// NewWithRunner constructs an Escalator with the provided Runner.
// Nil fields default to exec.CommandContext and exec.LookPath.
func NewWithRunner(r *Runner) Escalator {
	cmd := Commander(commanderFunc(exec.CommandContext))
	lp := exec.LookPath
	if r != nil {
		if r.Cmd != nil {
			cmd = r.Cmd
		}
		if r.LookPath != nil {
			lp = r.LookPath
		}
	}
	return &sudoEscalator{useSudo: !HasCaps(), runner: &Runner{Cmd: cmd, LookPath: lp}}
}
