//go:build integration

package integration

import (
	"os/exec"
	"testing"
)

func TestResumeAfterRebuild(t *testing.T) {
	requireRootAndCommands(t)
	cmd := exec.Command("bash", "./integration/resume_rebuild.sh")
	cmd.Dir = ".."
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("resume after rebuild failed: %v\n%s", err, out)
	}
}
