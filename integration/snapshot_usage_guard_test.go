//go:build integration

package integration

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"testing"
	"time"

	"lvmsync_go/internal/exitcode"
)

func run(t *testing.T, cmd *exec.Cmd) string {
	t.Helper()
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%v failed: %v\n%s", cmd.Args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func waitForSnapshot(t *testing.T, vg string) {
	for i := 0; i < 100; i++ {
		out := run(t, exec.Command("lvs", "--noheadings", "-o", "lv_name", vg))
		for _, f := range strings.Fields(out) {
			if strings.HasPrefix(f, "lvmsync_") {
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("snapshot not created")
}

func TestSnapshotUsageGuard(t *testing.T) {
	requireRootAndCommands(t, "pvcreate", "vgcreate", "lvcreate", "dd", "lvs")

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

	cmd := exec.Command(bin, "run", "--speed=1M", "--snapshot-size=1M", "--snapshot-max-usage=50", "/dev/vgtest/origin", "/dev/vgtest/dest")
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start lvmsync: %v", err)
	}

	waitForSnapshot(t, "vgtest")
	if err := cmd.Process.Signal(syscall.SIGSTOP); err != nil {
		t.Fatalf("failed to stop process: %v", err)
	}
	run(t, exec.Command("dd", "if=/dev/zero", "of=/dev/vgtest/origin", "bs=1M", "count=2", "conv=notrunc"))
	if err := cmd.Process.Signal(syscall.SIGCONT); err != nil {
		t.Fatalf("failed to continue process: %v", err)
	}

	err := cmd.Wait()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected exit error, got %v", err)
	}
	if exitErr.ExitCode() != exitcode.SnapshotExhausted {
		t.Fatalf("expected exit code %d, got %d", exitcode.SnapshotExhausted, exitErr.ExitCode())
	}

	out := run(t, exec.Command("lvs", "--noheadings", "-o", "lv_name", "vgtest"))
	fields := strings.Fields(out)
	sort.Strings(fields)
	if len(fields) != 2 || fields[0] != "dest" || fields[1] != "origin" {
		t.Fatalf("unexpected volumes: %v", fields)
	}
}
