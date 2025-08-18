//go:build integration

package integration

import (
	"os/exec"
	"testing"
)

func TestVerifyOnly(t *testing.T) {
	cmd := exec.Command("bash", "./integration/verify.sh")
	cmd.Dir = ".."
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("verify integration failed: %v\n%s", err, out)
	}
}
