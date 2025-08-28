//go:build integration

package verify

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/oferchen/lvmsync_go/internal/exitcode"
)

func TestRunVerifyOnlyMismatchExitCode(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root")
	}

	dir := t.TempDir()
	srcFile := filepath.Join(dir, "src.img")
	dstFile := filepath.Join(dir, "dst.img")

	block := bytes.Repeat([]byte{1}, 4096)
	if err := os.WriteFile(srcFile, block, 0o600); err != nil {
		t.Fatalf("write src: %v", err)
	}
	dstData := append([]byte{}, block...)
	dstData[0] = 2
	if err := os.WriteFile(dstFile, dstData, 0o600); err != nil {
		t.Fatalf("write dst: %v", err)
	}

	srcLoop := setupLoop(t, srcFile)
	dstLoop := setupLoop(t, dstFile)

	bin := filepath.Join(dir, "lvmsync")
	build := exec.Command("go", "build", "-o", bin, ".")
	build.Dir = filepath.Join("..", "..")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build lvmsync: %v\n%s", err, out)
	}

	cmd := exec.Command(bin, "run", "--verify-only", srcLoop, dstLoop)
	err := cmd.Run()
	if err == nil {
		t.Fatalf("expected verify-only to fail")
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("verify-only: %v", err)
	}
	if exitErr.ExitCode() != exitcode.Verify {
		t.Fatalf("expected exit code %d, got %d", exitcode.Verify, exitErr.ExitCode())
	}
}
