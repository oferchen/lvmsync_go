//go:build integration

package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestRsyncRefusal exercises rsync transport refusal and related warnings.
func TestRsyncRefusal(t *testing.T) {
	tmp := t.TempDir()

	// Build lvmsync binary into the temp directory.
	bin := filepath.Join(tmp, "lvmsync")
	build := exec.Command("go", "build", "-o", bin, ".")
	build.Dir = ".."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build lvmsync: %v\n%s", err, out)
	}

	if _, err := exec.LookPath("lvs"); err != nil {
		t.Skip("lvs not available")
	}

	// Prepare a 1MiB source image and matching destinations.
	data := make([]byte, 1<<20)
	src := filepath.Join(tmp, "src.img")
	if err := os.WriteFile(src, data, 0o600); err != nil {
		t.Fatalf("write src: %v", err)
	}

	dstNeg := filepath.Join(tmp, "dst_neg.img")
	dstPos := filepath.Join(tmp, "dst_pos.img")
	dstMis := filepath.Join(tmp, "dst_mis.img")
	for _, dst := range []string{dstNeg, dstPos, dstMis} {
		if err := os.WriteFile(dst, data, 0o600); err != nil {
			t.Fatalf("write %s: %v", dst, err)
		}
	}

	t.Run("refuses without allow-insecure", func(t *testing.T) {
		cmd := exec.Command(bin, "--transport=rsync", "--force", "--allow-overwrite", "--yes-i-know", "--skip-snapshot-creation", "--source-type=file", src, dstNeg)
		cmd.Dir = tmp
		cmd.Env = append(os.Environ(), "LVMSYNC_SNAPSHOT_SIZE=0", "LVMSYNC_SKIP_SNAPSHOT_CREATION=true")
		out, err := cmd.CombinedOutput()
		if err == nil {
			t.Fatalf("expected failure without --allow-insecure\n%s", out)
		}
		if !strings.Contains(string(out), "unsupported transport") {
			t.Fatalf("missing unsupported transport warning: %s", out)
		}
	})

	t.Run("warns and succeeds with allow-insecure", func(t *testing.T) {
		cmd := exec.Command(bin, "--transport=rsync", "--allow-insecure", "--force", "--allow-overwrite", "--yes-i-know", "--skip-snapshot-creation", "--source-type=file", src, dstPos)
		cmd.Dir = tmp
		cmd.Env = append(os.Environ(), "LVMSYNC_SNAPSHOT_SIZE=0", "LVMSYNC_SKIP_SNAPSHOT_CREATION=true")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("expected success with --allow-insecure: %v\n%s", err, out)
		}
		if !strings.Contains(string(out), "plaintext_connection") {
			t.Fatalf("missing plaintext warning: %s", out)
		}
	})

	t.Run("precondition failure on identity mismatch", func(t *testing.T) {
		// ensure destination matches source before mutation
		if err := os.WriteFile(dstMis, data, 0o600); err != nil {
			t.Fatalf("rewrite dst: %v", err)
		}
		go func() {
			time.Sleep(100 * time.Millisecond)
			_ = os.Truncate(dstMis, 0)
		}()

		cmd := exec.Command(bin, "--transport=rsync", "--allow-insecure", "--force", "--allow-overwrite", "--yes-i-know", "--skip-snapshot-creation", "--source-type=file", src, dstMis)
		cmd.Dir = tmp
		cmd.Env = append(os.Environ(), "LVMSYNC_SNAPSHOT_SIZE=0", "LVMSYNC_SKIP_SNAPSHOT_CREATION=true")
		out, err := cmd.CombinedOutput()
		if err == nil {
			t.Fatalf("expected precondition failure\n%s", out)
		}
		ee, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("unexpected error type: %T\n%s", err, out)
		}
		if ee.ExitCode() != 80 {
			t.Fatalf("expected exit code 80, got %d\n%s", ee.ExitCode(), out)
		}
		if !strings.Contains(string(out), "precondition") {
			t.Fatalf("missing precondition message: %s", out)
		}
	})
}
