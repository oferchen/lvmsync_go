//go:build integration

package integration

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"lvmsync_go/internal/exitcode"
)

// run executes the command and fails the test on error, returning trimmed output.
func run(t *testing.T, cmd *exec.Cmd) string {
	t.Helper()
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%v failed: %v\n%s", cmd.Args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestSnapshotPressure(t *testing.T) {
	// Require necessary LVM tools
	for _, tool := range []string{"losetup", "pvcreate", "vgcreate", "lvcreate", "dd"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s not available", tool)
		}
	}
	if os.Geteuid() != 0 {
		t.Skip("root required")
	}

	tmpDir := t.TempDir()
	bin := filepath.Join(tmpDir, "lvmsync")
	build := exec.Command("go", "build", "-o", bin, ".")
	build.Dir = ".."
	run(t, build)

	img := filepath.Join(tmpDir, "disk.img")
	run(t, exec.Command("dd", "if=/dev/zero", "of="+img, "bs=1M", "count=128"))
	loop := run(t, exec.Command("losetup", "--find", "--show", img))

	cleanup := func() {
		exec.Command("bash", "-c", "lvremove -f vgtest/* >/dev/null 2>&1").Run()
		exec.Command("vgremove", "-f", "vgtest").Run()
		exec.Command("pvremove", "-ff", loop).Run()
		exec.Command("losetup", "-d", loop).Run()
	}
	defer cleanup()

	run(t, exec.Command("pvcreate", "-ffy", loop))
	run(t, exec.Command("vgcreate", "vgtest", loop))
	run(t, exec.Command("lvcreate", "-n", "origin", "-L", "64M", "vgtest"))
	run(t, exec.Command("dd", "if=/dev/urandom", "of=/dev/vgtest/origin", "bs=1M", "count=64"))
	run(t, exec.Command("lvcreate", "-n", "dest", "-L", "64M", "vgtest"))

	cmd := exec.Command(bin, "run", "--speed=1M", "--snapshot-size=1M", "/dev/vgtest/origin", "/dev/vgtest/dest")
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start lvmsync: %v", err)
	}

	// Allow snapshot creation before modifying origin
	time.Sleep(2 * time.Second)
	run(t, exec.Command("dd", "if=/dev/zero", "of=/dev/vgtest/origin", "bs=1M", "count=8", "conv=notrunc"))

	err := cmd.Wait()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected exit error, got %v", err)
	}
	if exitErr.ExitCode() != exitcode.ErrDevice {
		t.Fatalf("expected exit code %d, got %d", exitcode.ErrDevice, exitErr.ExitCode())
	}
}
