// Package privilege provides helpers for validating and executing commands
// with the required elevated permissions. It prefers Linux capabilities and
// falls back to sudo when capabilities are missing.
package privilege

import "os/exec"

// Escalator validates privilege requirements and executes commands with
// escalation when needed.
type Escalator interface {
	// Ensure verifies that the required capabilities or sudo are available.
	Ensure() error
	// Command constructs an *exec.Cmd, adding sudo when capabilities are
	// insufficient.
	Command(name string, args ...string) *exec.Cmd
}

// sudoEscalator implements Escalator using Linux capabilities when present
// and sudo -n as a fallback.
type sudoEscalator struct{ useSudo bool }

// New returns an Escalator. If the current process lacks the required
// capabilities, commands will be executed via sudo -n.
func New() Escalator { return &sudoEscalator{useSudo: !hasCaps()} }
