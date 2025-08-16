// Package privilege contains a sudo-based escalator used when capabilities are
// unavailable.
package privilege

import (
	"fmt"
	"os/exec"
)

var lookPath = exec.LookPath

// Ensure verifies that either the required capabilities are present or that
// sudo is available when escalation is necessary.
func (s *sudoEscalator) Ensure() error {
	if s.useSudo {
		if _, err := lookPath("sudo"); err != nil {
			return fmt.Errorf("sudo not found: %w", err)
		}
		if err := execCommand("sudo", "-n", "true").Run(); err != nil {
			return fmt.Errorf("sudo escalation failed: %w", err)
		}
		return nil
	}
	return checkCaps()
}

// Command returns an *exec.Cmd that runs the given program. sudo -n is inserted
// when capabilities are missing.
func (s *sudoEscalator) Command(name string, args ...string) *exec.Cmd {
	if s.useSudo {
		all := append([]string{"-n", name}, args...)
		return execCommand("sudo", all...)
	}
	return execCommand(name, args...)
}
