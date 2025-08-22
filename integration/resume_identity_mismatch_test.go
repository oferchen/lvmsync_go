//go:build integration

package integration

import (
	"os/exec"
	"testing"
)

func TestResumeIdentityMismatch(t *testing.T) {
	requireRootAndCommands(t)
	cmd := exec.Command("bash", "./integration/resume_identity_mismatch.sh")
	cmd.Dir = ".."
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("identity mismatch integration failed: %v\n%s", err, out)
	}
}
