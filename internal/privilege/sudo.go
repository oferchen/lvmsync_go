// Package privilege contains a sudo-based escalator used when capabilities are
// unavailable.
package privilege

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"go.uber.org/zap"
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
		cmd := s.runner.Cmd.CommandContext(ctx, "sudo", "-n", "true")
		if s.sanitizeEnv {
			environ := os.Environ
			if s.environ != nil {
				environ = s.environ
			}
			cmd.Env = sanitizeEnv(environ())
		}
		if err := cmd.Run(); err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return fmt.Errorf("sudo escalation failed: %w", ctxErr)
			}
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
	s.runner.Logger.Debug("exec_command",
		zap.String("command", name),
		zap.Strings("args", redactArgs(args)),
	)
	if s.useSudo {
		all := append([]string{"-n", name}, args...)
		cmd := s.runner.Cmd.CommandContext(ctx, "sudo", all...)
		if s.sanitizeEnv {
			environ := os.Environ
			if s.environ != nil {
				environ = s.environ
			}
			cmd.Env = sanitizeEnv(environ())
		}
		return cmd
	}
	cmd := s.runner.Cmd.CommandContext(ctx, name, args...)
	if s.sanitizeEnv {
		environ := os.Environ
		if s.environ != nil {
			environ = s.environ
		}
		cmd.Env = sanitizeEnv(environ())
	}
	return cmd
}

// sanitizeEnv drops unsafe variables like PATH, LANG, and anything starting
// with LD_. Only a small whitelist of locale-related variables is preserved.
func sanitizeEnv(environ []string) []string {
	whitelist := map[string]bool{
		"LC_ALL":   true,
		"LC_CTYPE": true,
		"TERM":     true,
	}
	out := make([]string, 0, len(environ))
	for _, kv := range environ {
		if strings.HasPrefix(kv, "LD_") || strings.HasPrefix(kv, "GCONV_PATH=") {
			continue
		}
		i := strings.IndexByte(kv, '=')
		if i <= 0 {
			continue
		}
		k := kv[:i]
		if whitelist[k] {
			out = append(out, kv)
		}
	}
	return out
}

func redactArgs(args []string) []string {
	out := make([]string, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		lower := strings.ToLower(a)
		if strings.Contains(lower, "password") || strings.Contains(lower, "secret") ||
			strings.Contains(lower, "token") || strings.Contains(lower, "key") {
			if strings.Contains(a, "=") {
				parts := strings.SplitN(a, "=", 2)
				out[i] = parts[0] + "=[REDACTED]"
			} else {
				out[i] = a
				if i+1 < len(args) {
					out[i+1] = "[REDACTED]"
					i++
				}
			}
		} else {
			out[i] = a
		}
	}
	return out
}
