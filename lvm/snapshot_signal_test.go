package lvm

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"go.uber.org/zap"
)

func setupLoop(t *testing.T, size int64) (string, func()) {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "loop-*.img")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	if err := f.Truncate(size); err != nil {
		f.Close()
		t.Fatalf("truncate: %v", err)
	}
	f.Close()
	out, err := exec.Command("losetup", "--show", "-f", f.Name()).Output()
	if err != nil {
		t.Skipf("losetup: %v", err)
	}
	loop := strings.TrimSpace(string(out))
	cleanup := func() { _ = exec.Command("losetup", "-d", loop).Run() }
	return loop, cleanup
}

func run(t *testing.T, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("%s %v: %v\n%s", name, args, err, out)
	}
}

func TestSnapshotRemovedOnSignal(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root")
	}

	loop, loopCleanup := setupLoop(t, 64<<20)
	run(t, "pvcreate", "-ffy", loop)
	run(t, "vgcreate", "vgsig", loop)
	run(t, "lvcreate", "-n", "origin", "-L", "32M", "vgsig")

	t.Cleanup(func() {
		exec.Command("lvremove", "-f", "vgsig/snap").Run()
		exec.Command("lvremove", "-f", "vgsig/origin").Run()
		exec.Command("vgremove", "-f", "vgsig").Run()
		exec.Command("pvremove", "-ff", loop).Run()
		loopCleanup()
	})

	if err := CreateSnapshot(context.Background(), "/dev/vgsig/origin", "snap", "8M", zap.NewNop()); err != nil {
		t.Skipf("CreateSnapshot: %v", err)
	}
	snapPath := "/dev/vgsig/snap"
	sr := NewSnapshotRegistry(nil)
	sr.RegisterSnapshot(snapPath, zap.NewNop())

	if err := syscall.Kill(os.Getpid(), syscall.SIGINT); err != nil {
		t.Fatalf("send signal: %v", err)
	}

	time.Sleep(2 * time.Second)
	if err := exec.Command("lvs", snapPath).Run(); err == nil {
		t.Fatalf("snapshot %s still exists", snapPath)
	}
}
