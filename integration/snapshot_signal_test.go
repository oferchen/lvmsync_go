//go:build integration

package integration

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
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

func TestSnapshotCleanupOnSignal(t *testing.T) {
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
		exec.Command("bash", "-c", "lvremove -f vgsig/* >/dev/null 2>&1").Run()
		exec.Command("vgremove", "-f", "vgsig").Run()
		exec.Command("pvremove", "-ff", loop).Run()
		exec.Command("losetup", "-d", loop).Run()
	}
	defer cleanup()

	run(t, exec.Command("pvcreate", "-ffy", loop))
	run(t, exec.Command("vgcreate", "vgsig", loop))
	run(t, exec.Command("lvcreate", "-n", "origin", "-L", "32M", "vgsig"))
	run(t, exec.Command("dd", "if=/dev/urandom", "of=/dev/vgsig/origin", "bs=1M", "count=32"))

	cmd := exec.Command(bin, "run", "--speed=1M", "--snapshot-size=1M", "--create-dest-lv", "/dev/vgsig/origin", "/dev/vgsig/dest")
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start lvmsync: %v", err)
	}

	for i := 0; i < 20; i++ {
		if exec.Command("lvs", "/dev/vgsig/dest").Run() == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if err := cmd.Process.Signal(syscall.SIGINT); err != nil {
		t.Fatalf("send signal: %v", err)
	}
	err := cmd.Wait()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected exit error, got %v", err)
	}

	out := run(t, exec.Command("lvs", "--noheadings", "-o", "lv_name", "vgsig"))
	fields := strings.Fields(out)
	if len(fields) != 2 {
		t.Fatalf("expected 2 logical volumes, got %d: %s", len(fields), out)
	}
	if !(fields[0] == "dest" && fields[1] == "origin" || fields[0] == "origin" && fields[1] == "dest") {
		t.Fatalf("unexpected volumes: %v", fields)
	}
}
