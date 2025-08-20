package privilege

import (
	"context"
	"os"
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
// ctx is currently unused but reserved for future use.
func New(ctx context.Context) Escalator { return newEscalator(ctx, nil, false) }

// NewWithRunner constructs an Escalator with the provided Runner.
// Nil fields default to exec.CommandContext and exec.LookPath.
func NewWithRunner(ctx context.Context, r *Runner) Escalator {
	return newEscalator(ctx, r, false)
}

// NewWithSanitize constructs an Escalator that optionally sanitizes the
// environment of executed commands.
func NewWithSanitize(ctx context.Context, sanitize bool) Escalator {
	return newEscalator(ctx, nil, sanitize)
}

// NewWithRunnerAndSanitize constructs an Escalator with the provided Runner
// and optional environment sanitization.
func NewWithRunnerAndSanitize(ctx context.Context, r *Runner, sanitize bool) Escalator {
	return newEscalator(ctx, r, sanitize)
}

func newEscalator(_ context.Context, r *Runner, sanitize bool) Escalator {
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
	return &sudoEscalator{
		useSudo:     !HasCaps(),
		runner:      &Runner{Cmd: cmd, LookPath: lp},
		sanitizeEnv: sanitize,
		environ:     os.Environ,
	}
}
