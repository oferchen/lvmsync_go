//go:build integration

package integration

import (
	"os/exec"
	"testing"
)

func TestOfflineEnforcement(t *testing.T) {
	requireRootAndCommands(t)
	cmd := exec.Command("bash", "./integration/offline_enforcement.sh")
	cmd.Dir = ".."
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("offline enforcement integration failed: %v\n%s", err, out)
	}
}
