package lvm

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// execCommand is used for running external commands and can be overridden in tests.
var execCommand = exec.Command

// verifyEscalation checks that the escalation command succeeds when not running as root.
func verifyEscalation(escalation string) error {
	if os.Geteuid() == 0 {
		return nil
	}
	parts := strings.Fields(escalation)
	if len(parts) == 0 {
		return fmt.Errorf("insufficient privileges: LVM operations require root privileges")
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
