package privesc

import (
	"fmt"
	"os"
	"strings"
)

// ExecFunc models the signature of unix.Exec and allows tests to stub the
// exec call.
type ExecFunc func(argv0 string, argv, envv []string) error

// EnsureRoot verifies that the current process has root privileges. If not, it
// attempts to re-exec the process using the provided escalation command (e.g.
// "sudo -n"). The current process will be replaced on success. The execFunc
// parameter is injected to facilitate testing and typically wraps unix.Exec.
func EnsureRoot(escalationCmd string, execFunc ExecFunc) error {
	if os.Geteuid() == 0 {
		return nil
	}
	if strings.TrimSpace(escalationCmd) == "" {
		return fmt.Errorf("root privileges required")
	}
	parts := strings.Fields(escalationCmd)
	argv := append(append([]string{}, parts...), os.Args...)
	if err := execFunc(parts[0], argv, os.Environ()); err != nil {
		return fmt.Errorf("failed to exec %s: %w", parts[0], err)
	}
	return nil
}
