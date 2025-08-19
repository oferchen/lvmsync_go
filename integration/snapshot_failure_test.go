//go:build integration

package integration

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
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

func TestSnapshotCleanupOnFailure(t *testing.T) {
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
		exec.Command("bash", "-c", "lvremove -f vgfail/* >/dev/null 2>&1").Run()
		exec.Command("vgremove", "-f", "vgfail").Run()
		exec.Command("pvremove", "-ff", loop).Run()
		exec.Command("losetup", "-d", loop).Run()
	}
	defer cleanup()

	run(t, exec.Command("pvcreate", "-ffy", loop))
	run(t, exec.Command("vgcreate", "vgfail", loop))
	run(t, exec.Command("lvcreate", "-n", "origin", "-L", "32M", "vgfail"))
	run(t, exec.Command("dd", "if=/dev/urandom", "of=/dev/vgfail/origin", "bs=1M", "count=32"))

	cmd := exec.Command(bin, "run", "--speed=1M", "--snapshot-size=1M", "--create-dest-lv", "/dev/vgfail/origin", "/dev/vgmissing/dest")
	err := cmd.Run()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected exit error, got %v", err)
	}

	out := run(t, exec.Command("lvs", "--noheadings", "-o", "lv_name", "vgfail"))
	fields := strings.Fields(out)
	if len(fields) != 1 || fields[0] != "origin" {
		t.Fatalf("unexpected volumes: %v", fields)
	}
}
