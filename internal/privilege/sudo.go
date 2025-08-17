// Package privilege contains a sudo-based escalator used when capabilities are
// unavailable.
package privilege

import (
	"context"
	"fmt"
	"os/exec"
	"time"
)

// escalationTimeout limits how long privilege checks and escalated commands may run
// when the caller does not provide a deadline.
var escalationTimeout = 5 * time.Second

func withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if _, ok := ctx.Deadline(); ok {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, escalationTimeout)
}

// Ensure verifies that either the required capabilities are present or that
// sudo is available when escalation is necessary.
func (s *sudoEscalator) Ensure(ctx context.Context) error {
	ctx, cancel := withTimeout(ctx)
	defer cancel()
	if s.useSudo {
		if _, err := s.runner.LookPath("sudo"); err != nil {
			return fmt.Errorf("sudo not found: %w", err)
		}
		if err := s.runner.Cmd.CommandContext(ctx, "sudo", "-n", "true").Run(); err != nil {
			return fmt.Errorf("sudo escalation failed: %w", err)
		}
		return nil
	}
	return checkCaps()
}

// Command returns an *exec.Cmd that runs the given program. sudo -n is inserted
// when capabilities are missing.
func (s *sudoEscalator) Command(ctx context.Context, name string, args ...string) *exec.Cmd {
	ctx, _ = withTimeout(ctx)
	if s.useSudo {
		all := append([]string{"-n", name}, args...)
		return s.runner.Cmd.CommandContext(ctx, "sudo", all...)
	}
	return s.runner.Cmd.CommandContext(ctx, name, args...)
}
