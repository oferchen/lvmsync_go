//go:build integration

package integration

import (
	"os/exec"
	"testing"
)

func TestSafeOverwrite(t *testing.T) {
	cmd := exec.Command("bash", "./integration/safe_overwrite.sh")
	cmd.Dir = ".."
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("safe-overwrite integration failed: %v\n%s", err, out)
	}
}
