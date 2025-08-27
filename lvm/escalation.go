package lvm

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/kballard/go-shellquote"
)

// EscalationChecker validates that an escalation command is available.
type EscalationChecker struct {
	execCommand func(string, ...string) *exec.Cmd
	geteuid     func() int
}

// NewEscalationChecker returns an EscalationChecker using real OS functions.
func NewEscalationChecker() *EscalationChecker {
	return NewEscalationCheckerWithDeps(exec.Command, os.Geteuid)
}

// NewEscalationCheckerWithDeps allows tests to inject dependencies.
func NewEscalationCheckerWithDeps(execCommand func(string, ...string) *exec.Cmd, geteuid func() int) *EscalationChecker {
	if execCommand == nil {
		execCommand = exec.Command
	}
	if geteuid == nil {
		geteuid = os.Geteuid
	}
	return &EscalationChecker{execCommand: execCommand, geteuid: geteuid}
}

// ParseEscalation splits the escalation command using shell-style quoting and
// rejects unsafe characters. It returns an error if the command is empty or
// contains unsupported tokens like pipes or redirects.
func ParseEscalation(escalation string) ([]string, error) {
	if strings.TrimSpace(escalation) == "" {
		return nil, fmt.Errorf("lvm escalation command is empty")
	}
	if strings.ContainsAny(escalation, "|&;<>") {
		return nil, fmt.Errorf("unsupported characters in lvm escalation command")
	}
	parts, err := shellquote.Split(escalation)
	if err != nil {
		return nil, fmt.Errorf("invalid lvm escalation command: %w", err)
	}
	if len(parts) == 0 {
		return nil, fmt.Errorf("lvm escalation command is empty")
	}
	return parts, nil
}

// verifyEscalation checks that the escalation command succeeds when not running as root.
func (e *EscalationChecker) verifyEscalation(escalation string) error {
	if e.geteuid() == 0 {
		return nil
	}
	parts, err := ParseEscalation(escalation)
	if err != nil {
		return fmt.Errorf("insufficient privileges: %w", err)
	}
	cmd := e.execCommand(parts[0], append(parts[1:], "true")...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("escalation command failed: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// VerifyEscalationCommand validates that the escalation command is available.
func (e *EscalationChecker) VerifyEscalationCommand(escalation string) error {
	return e.verifyEscalation(escalation)
}

var defaultEscalationChecker = NewEscalationChecker()

// VerifyEscalationCommand validates that the escalation command is available using
// the default EscalationChecker instance.
func VerifyEscalationCommand(escalation string) error {
	return defaultEscalationChecker.VerifyEscalationCommand(escalation)
}

// SetEscalationChecker overrides the default EscalationChecker. It returns a
// restore function to reset the original behavior.
func SetEscalationChecker(c *EscalationChecker) func() {
	orig := defaultEscalationChecker
	if c == nil {
		defaultEscalationChecker = NewEscalationChecker()
	} else {
		defaultEscalationChecker = c
	}
	return func() { defaultEscalationChecker = orig }
}
