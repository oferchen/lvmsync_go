package privesc

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/sys/unix"
)

// EnsureRoot verifies that the current process has root privileges.
// If not, it attempts to re-exec the process using the provided
// escalation command (e.g. "sudo -n"). The current process will be
// replaced on success.
func EnsureRoot(escalationCmd string) error {
	if os.Geteuid() == 0 {
		return nil
	}
	if strings.TrimSpace(escalationCmd) == "" {
		return fmt.Errorf("root privileges required")
	}
	parts := strings.Fields(escalationCmd)
	argv := append(append([]string{}, parts...), os.Args...)
	if err := unix.Exec(parts[0], argv, os.Environ()); err != nil {
		return fmt.Errorf("failed to exec %s: %w", parts[0], err)
	}
	return nil
}
