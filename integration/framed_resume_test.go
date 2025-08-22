//go:build integration

package integration

import (
	"os/exec"
	"testing"
)

func TestFramedResumeWithCompression(t *testing.T) {
	requireRootAndCommands(t)
	cmd := exec.Command("bash", "./integration/framed_resume.sh")
	cmd.Dir = ".."
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("framed resume integration failed: %v\n%s", err, out)
	}
}
