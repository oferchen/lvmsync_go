//go:build linux

package device

import (
	"context"
	"os"
	"os/exec"
	"reflect"
	"testing"

	"go.uber.org/zap"

	"lvmsync_go/internal/lock"
	"lvmsync_go/lvm"
)

func TestOpenLVM(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root")
	}
	restoreDir := lock.SetBaseDir(t.TempDir())
	t.Cleanup(restoreDir)
	loop, cleanup := setupLoop(t, 1<<20)
	defer cleanup()
	cache := lvm.NewDeviceFDCache(zap.NewNop())
	defer cache.Close()
	runner := NewRunner()
	dev, err := runner.OpenLVM(loop, cache, "", zap.NewNop())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if dev.Path() != loop {
		t.Fatalf("path = %s, want %s", dev.Path(), loop)
	}
	if dev.SizeBytes() == 0 || dev.BlockSize() == 0 {
		t.Fatalf("unexpected size or block size")
	}
	dev.Close()
}

func TestOpenLVMNonBlockDevice(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "file")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	f.Close()
	cache := lvm.NewDeviceFDCache(zap.NewNop())
	defer cache.Close()
	runner := NewRunner()
	if _, err := runner.OpenLVM(f.Name(), cache, "", zap.NewNop()); err == nil {
		t.Fatalf("expected error for non-block device")
	}
}

func TestOpenLVMChecks(t *testing.T) {
	cache := lvm.NewDeviceFDCache(zap.NewNop())
	defer cache.Close()
	runner := NewRunnerWithDeps(func(context.Context, string) (bool, error) { return false, nil }, lvm.AutoExtendEnabled, lvm.DiscardEnabled, defaultIsMountedRW, lock.Acquire, nil)
	if _, err := runner.OpenLVM("/dev/missing", cache, "", zap.NewNop()); err == nil {
		t.Fatalf("expected error when volume missing")
	}
}

func TestRunLVMPrivilegeEscalation(t *testing.T) {
	ctx := context.Background()
	restore := lvm.SetEscalationChecker(func(string) error { return nil })
	defer restore()
	var gotName string
	var gotArgs []string
	cmd := cmdFunc(func(ctx context.Context, name string, args ...string) *exec.Cmd {
		gotName = name
		gotArgs = append([]string(nil), args...)
		return exec.CommandContext(ctx, "true")
	})
	runner := NewDeviceRunner(cmd)
	origEuid := geteuid
	geteuid = func() int { return 1 }
	t.Cleanup(func() { geteuid = origEuid })
	if err := runner.runLVM(ctx, "doas -n", "lvremove", "-f", "/dev/vg0/snap"); err != nil {
		t.Fatalf("runLVM: %v", err)
	}
	if gotName != "doas" || !reflect.DeepEqual(gotArgs, []string{"-n", "lvremove", "-f", "/dev/vg0/snap"}) {
		t.Fatalf("unexpected command: %s %v", gotName, gotArgs)
	}
}

func TestRunLVMFailure(t *testing.T) {
	ctx := context.Background()
	cmd := cmdFunc(func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "false")
	})
	runner := NewDeviceRunner(cmd)
	if err := runner.runLVM(ctx, "", "lvremove", "-f", "/dev/vg0/snap"); err == nil {
		t.Fatalf("expected error from runLVM")
	}
}
