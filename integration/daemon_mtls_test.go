//go:build integration

package integration

import (
	"os/exec"
	"testing"
)

func TestDaemonMTLS(t *testing.T) {
	requireRootAndCommands(t)
	cmd := exec.Command("bash", "./integration/daemon_mtls.sh")
	cmd.Dir = ".."
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("daemon mTLS integration failed: %v\n%s", err, out)
	}
}
