package privilege

import (
        "context"
        "os/exec"

        "go.uber.org/zap"
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
	Logger   *zap.Logger
}

// New returns an Escalator with production dependencies.
// ctx is currently unused but reserved for future use.
func New(ctx context.Context, logger *zap.Logger) Escalator {
        return NewWithRunner(ctx, nil, logger)
}

// NewWithRunner constructs an Escalator with the provided Runner.
// Nil fields default to exec.CommandContext and exec.LookPath.
func NewWithRunner(ctx context.Context, r *Runner, logger *zap.Logger) Escalator {
        if logger == nil {
                logger = zap.NewNop()
        }
        if r == nil {
                r = &Runner{Logger: logger}
        } else if r.Logger == nil {
                r.Logger = logger
        }
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
        logger := zap.NewNop()
        if r != nil {
                if r.Cmd != nil {
                        cmd = r.Cmd
                }
                if r.LookPath != nil {
                        lp = r.LookPath
                }
                if r.Logger != nil {
                        logger = r.Logger
                }
        }
        return &sudoEscalator{
                useSudo:     !HasCaps(),
                runner:      &Runner{Cmd: cmd, LookPath: lp, Logger: logger},
                sanitizeEnv: sanitize,
        }
}
