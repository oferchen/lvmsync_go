//go:build linux

package device

import (
	"context"
	"os"
	"os/exec"
	"reflect"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

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
	runner := NewRunnerWithDeps(func(context.Context, string) (bool, error) { return false, nil }, lvm.AutoExtendEnabled, lvm.DiscardEnabled, IsMountedRW, lock.Acquire, nil)
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

func TestLVMDeviceCloseReleasesLock(t *testing.T) {
	restoreDir := lock.SetBaseDir(t.TempDir())
	t.Cleanup(restoreDir)

	f, err := os.CreateTemp(t.TempDir(), "lvmdev")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	lk, err := lock.Acquire("vg", "lv")
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	d := &LVMDevice{f: f, path: f.Name(), lock: lk, logger: zap.NewNop()}
	if err := d.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := lock.Acquire("vg", "lv"); err != nil {
		t.Fatalf("lock not released: %v", err)
	}
}

func TestLVMDeviceCloseErrorLogging(t *testing.T) {
	restoreDir := lock.SetBaseDir(t.TempDir())
	t.Cleanup(restoreDir)

	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core)

	f, err := os.CreateTemp(t.TempDir(), "lvmdev")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	lk, err := lock.Acquire("vg", "lv")
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("preclose: %v", err)
	}
	d := &LVMDevice{f: f, path: f.Name(), lock: lk, logger: logger}
	if err := d.Close(); err == nil {
		t.Fatalf("expected error from Close")
	}
	if logs.FilterMessage("lvm_device_close_failed").Len() == 0 {
		t.Fatalf("expected lvm_device_close_failed log")
	}
}
