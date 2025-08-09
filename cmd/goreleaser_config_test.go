package cmd

import (
	"os/exec"
	"testing"
)

func TestGoreleaserConfig(t *testing.T) {
	cmd := exec.Command("goreleaser", "check")
	cmd.Dir = ".."
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("goreleaser check failed: %v\nOutput: %s", err, output)
	}
}
