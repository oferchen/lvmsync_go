//go:build integration

package integration

import (
	"os/exec"
	"path/filepath"
	"testing"
)

func TestFreezeHelperValidation(t *testing.T) {
	requireRootAndCommands(t)

	tmp := t.TempDir()
	bin := filepath.Join(tmp, "lvmsync")
	build := exec.Command("go", "build", "-o", bin, ".")
	build.Dir = ".."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build lvmsync: %v\n%s", err, out)
	}

	dest := filepath.Join(tmp, "dest")
	cmd := exec.Command(bin,
		"--dry-run",
		"--transport=rsync",
		"--force",
		"--allow-overwrite",
		"--yes-i-know",
		"--fs-freeze-command", "/does-not-exist",
		"--fs-thaw-command", "/bin/true",
		"/dev/null", dest,
	)
	cmd.Dir = tmp
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected failure with invalid freeze helper\n%s", out)
	}
	ee, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("unexpected error type: %T\n%s", err, out)
	}
	if ee.ExitCode() != 80 {
		t.Fatalf("expected exit code 80, got %d\n%s", ee.ExitCode(), out)
	}
}
