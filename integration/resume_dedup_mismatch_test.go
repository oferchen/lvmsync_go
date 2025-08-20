//go:build integration

package integration

import (
	"os/exec"
	"testing"
)

func TestResumeDedupMismatch(t *testing.T) {
	cmd := exec.Command("bash", "./integration/resume_dedup_mismatch.sh")
	cmd.Dir = ".."
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("dedup mismatch integration failed: %v\n%s", err, out)
	}
}
