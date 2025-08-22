//go:build integration

package integration

import (
	"os"
	"os/exec"
	"testing"
)

// requireRootAndCommands skips the test when run without root privileges or
// when required external commands are missing. The default checks cover common
// LVM utilities and loop device helpers; additional tools can be passed via
// cmds.
func requireRootAndCommands(t *testing.T, cmds ...string) {
	t.Helper()
	if os.Geteuid() != 0 {
		t.Skip("root required")
	}
	tools := append([]string{"lvremove", "pvremove", "losetup"}, cmds...)
	for _, tool := range tools {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s not available", tool)
		}
	}
}
