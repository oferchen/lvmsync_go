package lvm

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/kballard/go-shellquote"
)

// execCommand is used for running external commands and can be overridden in tests.
var execCommand = exec.Command

// geteuid returns the effective user ID; overridable for tests.
var geteuid = os.Geteuid

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
func verifyEscalation(escalation string) error {
	if geteuid() == 0 {
		return nil
	}
	parts, err := ParseEscalation(escalation)
	if err != nil {
		return fmt.Errorf("insufficient privileges: %w", err)
	}
	cmd := execCommand(parts[0], append(parts[1:], "true")...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("escalation command failed: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

var checkEscalation = verifyEscalation

// VerifyEscalationCommand validates that the escalation command is available.
func VerifyEscalationCommand(escalation string) error { return checkEscalation(escalation) }

// SetEscalationChecker overrides the default escalation check function. It returns
// a restore function to reset the original behavior.
func SetEscalationChecker(fn func(string) error) func() {
	orig := checkEscalation
	if fn == nil {
		checkEscalation = verifyEscalation
	} else {
		checkEscalation = fn
	}
	return func() { checkEscalation = orig }
}
