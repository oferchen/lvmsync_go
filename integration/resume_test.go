//go:build integration

package integration

import (
	"os/exec"
	"testing"
)

func TestResumeMidTransfer(t *testing.T) {
	requireRootAndCommands(t)
	cmd := exec.Command("bash", "./integration/resume.sh")
	cmd.Dir = ".."
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("resume integration failed: %v\n%s", err, out)
	}
}
