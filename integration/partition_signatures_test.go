//go:build integration

package integration

import (
	"os/exec"
	"testing"
)

func TestPartitionSignatures(t *testing.T) {
	requireRootAndCommands(t, "sfdisk")
	cmd := exec.Command("bash", "./integration/partition_signatures.sh")
	cmd.Dir = ".."
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("partition-signatures integration failed: %v\n%s", err, out)
	}
}
