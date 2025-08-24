// Package privilege provides helpers for validating and executing commands
// with the required elevated permissions. It prefers Linux capabilities and
// falls back to sudo when capabilities are missing.
package privilege

import (
	"context"
	"os/exec"
	"time"
)

// Escalator validates privilege requirements and executes commands with
// escalation when needed.
type Escalator interface {
	// Ensure verifies that the required capabilities or sudo are available.
	Ensure(context.Context) error
	// Command constructs an *exec.Cmd, adding sudo when capabilities are
	// insufficient.
	Command(context.Context, string, ...string) *exec.Cmd
}

// sudoEscalator implements Escalator using Linux capabilities when present
// and sudo -n as a fallback.
type sudoEscalator struct {
	ctx         context.Context
	useSudo     bool
	runner      *Runner
	sanitizeEnv bool
	environ     func() []string
	timeout     time.Duration
}
