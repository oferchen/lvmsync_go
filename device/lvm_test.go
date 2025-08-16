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
	dev, err := OpenLVM(loop, cache, "", zap.NewNop())
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
	if _, err := OpenLVM(f.Name(), cache, "", zap.NewNop()); err == nil {
		t.Fatalf("expected error for non-block device")
	}
}

func TestRunLVMPrivilegeEscalation(t *testing.T) {
	ctx := context.Background()
	var gotName string
	var gotArgs []string
	origExec := execCommand
	execCommand = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		gotName = name
		gotArgs = append([]string(nil), args...)
		return exec.CommandContext(ctx, "true")
	}
	t.Cleanup(func() { execCommand = origExec })
	origEuid := geteuid
	geteuid = func() int { return 1 }
	t.Cleanup(func() { geteuid = origEuid })
	if err := runLVM(ctx, "doas -n", "lvremove", "-f", "/dev/vg0/snap"); err != nil {
		t.Fatalf("runLVM: %v", err)
	}
	if gotName != "doas" || !reflect.DeepEqual(gotArgs, []string{"-n", "lvremove", "-f", "/dev/vg0/snap"}) {
		t.Fatalf("unexpected command: %s %v", gotName, gotArgs)
	}
}

func TestRunLVMFailure(t *testing.T) {
	ctx := context.Background()
	origExec := execCommand
	execCommand = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "false")
	}
	t.Cleanup(func() { execCommand = origExec })
	if err := runLVM(ctx, "", "lvremove", "-f", "/dev/vg0/snap"); err == nil {
		t.Fatalf("expected error from runLVM")
	}
}
