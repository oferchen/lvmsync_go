//go:build integration

package integration

import (
	"os/exec"
	"testing"
)

func TestLoopbackTransfer(t *testing.T) {
	requireRootAndCommands(t)
	cmd := exec.Command("bash", "./integration/loopback.sh")
	cmd.Dir = ".."
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("loopback integration failed: %v\n%s", err, out)
	}
}
