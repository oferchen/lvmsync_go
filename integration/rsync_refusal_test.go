//go:build integration

package integration

import (
	"os/exec"
	"testing"
)

func TestRsyncRefusal(t *testing.T) {
	cmd := exec.Command("bash", "./integration/rsync_refusal.sh")
	cmd.Dir = ".."
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("rsync refusal integration failed: %v\n%s", err, out)
	}
}
