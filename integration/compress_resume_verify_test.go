//go:build integration

package integration

import (
	"os/exec"
	"testing"
)

func TestCompressResumeVerify(t *testing.T) {
	requireRootAndCommands(t)
	cmd := exec.Command("bash", "./integration/compress_resume_verify.sh")
	cmd.Dir = ".."
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("compress resume verify failed: %v\n%s", err, out)
	}
}
