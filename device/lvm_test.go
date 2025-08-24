//go:build linux

package device

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"lvmsync_go/internal/exitcode"
	"lvmsync_go/internal/lock"
	"lvmsync_go/lvm"
)

type ttyLVMReader struct{ io.Reader }

func (t ttyLVMReader) Fd() uintptr { return 0 }

// vgSelectBackend simulates vgs output for volume group selection tests.
type vgSelectBackend struct {
	vgs map[string]uint64
}

func (b *vgSelectBackend) CreateSnapshot(context.Context, string, string, string) error { return nil }
func (b *vgSelectBackend) RemoveSnapshot(context.Context, string) error                 { return nil }
func (b *vgSelectBackend) GetSnapshotUsage(context.Context, string) (float64, error)    { return 0, nil }
func (b *vgSelectBackend) GetVolumeGroupFreeSpace(_ context.Context, name string) (uint64, error) {
	if free, ok := b.vgs[name]; ok {
		return free, nil
	}
	return 0, fmt.Errorf("device volume group %s not found", name)
}

func (b *vgSelectBackend) ListVolumeGroups(_ context.Context, candidates []string) ([]lvm.VolumeGroup, error) {
	if len(candidates) == 0 {
		res := []lvm.VolumeGroup{}
		for name, free := range b.vgs {
			res = append(res, lvm.VolumeGroup{Name: name, Free: free})
		}
		return res, nil
	}
	res := []lvm.VolumeGroup{}
	for _, c := range candidates {
		if free, ok := b.vgs[c]; ok {
			res = append(res, lvm.VolumeGroup{Name: c, Free: free})
		} else {
			return nil, fmt.Errorf("device volume group %s not found", c)
		}
	}
	return res, nil
}

func (b *vgSelectBackend) CreateLogicalVolume(context.Context, string, string, uint64) error {
	return nil
}

func TestOpenLVM(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root")
	}
	restoreDir := lock.SetBaseDir(t.TempDir())
	t.Cleanup(restoreDir)
	loop, cleanup := setupLoop(t, 1<<20)
	defer cleanup()
	cache, err := lvm.NewDeviceFDCache(zap.NewNop())
	if err != nil {
		t.Fatalf("NewDeviceFDCache: %v", err)
	}
	defer cache.Close()
	runner := NewRunner()
	dev, err := runner.OpenLVM(context.Background(), loop, cache, true, false, "", zap.NewNop())
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
	cache, err := lvm.NewDeviceFDCache(zap.NewNop())
	if err != nil {
		t.Fatalf("NewDeviceFDCache: %v", err)
	}
	defer cache.Close()
	runner := NewRunner()
	if _, err := runner.OpenLVM(context.Background(), f.Name(), cache, true, false, "", zap.NewNop()); err == nil {
		t.Fatalf("expected error for non-block device")
	}
}

func TestOpenLVMChecks(t *testing.T) {
	cache, err := lvm.NewDeviceFDCache(zap.NewNop())
	if err != nil {
		t.Fatalf("NewDeviceFDCache: %v", err)
	}
	defer cache.Close()
	runner := NewRunnerWithDeps(func(context.Context, string) (bool, error) { return false, nil }, lvm.AutoExtendEnabled, lvm.DiscardEnabled, defaultIsMountedRW, lock.Acquire, nil)
	if _, err := runner.OpenLVM(context.Background(), "/dev/missing", cache, true, false, "", zap.NewNop()); err == nil {
		t.Fatalf("expected error when volume missing")
	}
}

func TestOpenLVMRequiresSnapshot(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root")
	}
	loop, cleanup := setupLoop(t, 1<<20)
	defer cleanup()

	dir := t.TempDir()
	script := filepath.Join(dir, "lvs")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho '-wi-a----- linear'\n"), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	origPath, origErr := lvsPath, lvsErr
	lvsPath, lvsErr = script, nil
	t.Cleanup(func() { lvsPath, lvsErr = origPath, origErr })

	cache, err := lvm.NewDeviceFDCache(zap.NewNop())
	if err != nil {
		t.Fatalf("NewDeviceFDCache: %v", err)
	}
	defer cache.Close()
	runner := NewRunnerWithDeps(
		func(context.Context, string) (bool, error) { return true, nil },
		func(context.Context, string) (bool, error) { return false, nil },
		func(context.Context, string) (bool, error) { return true, nil },
		func(context.Context, string) (bool, error) { return false, nil },
		func(string, string) (*lock.Lock, error) { return &lock.Lock{}, nil },
		nil,
	)
	if _, err := runner.OpenLVM(context.Background(), loop, cache, true, false, "", zap.NewNop()); !errors.Is(err, exitcode.ErrPrecondition) {
		t.Fatalf("expected precondition error, got %v", err)
	}
	if _, err := runner.OpenLVM(context.Background(), loop, cache, true, true, "", zap.NewNop()); err != nil {
		t.Fatalf("offline open: %v", err)
	}
}

func TestOpenLVMSnapshotAllowed(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root")
	}
	loop, cleanup := setupLoop(t, 1<<20)
	defer cleanup()

	dir := t.TempDir()
	script := filepath.Join(dir, "lvs")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho 'swi-a-s--- snapshot'\n"), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	origPath, origErr := lvsPath, lvsErr
	lvsPath, lvsErr = script, nil
	t.Cleanup(func() { lvsPath, lvsErr = origPath, origErr })

	cache, err := lvm.NewDeviceFDCache(zap.NewNop())
	if err != nil {
		t.Fatalf("NewDeviceFDCache: %v", err)
	}
	defer cache.Close()
	runner := NewRunnerWithDeps(
		func(context.Context, string) (bool, error) { return true, nil },
		func(context.Context, string) (bool, error) { return false, nil },
		func(context.Context, string) (bool, error) { return true, nil },
		func(context.Context, string) (bool, error) { return false, nil },
		func(string, string) (*lock.Lock, error) { return &lock.Lock{}, nil },
		nil,
	)
	dev, err := runner.OpenLVM(context.Background(), loop, cache, true, false, "", zap.NewNop())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	dev.Close()
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

func TestRunLVMPrivilegeEscalationQuoted(t *testing.T) {
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
	esc := "\"/usr/bin/sudo wrapper\" -p 'my prompt' -n"
	if err := runner.runLVM(ctx, esc, "lvremove", "-f", "/dev/vg0/snap"); err != nil {
		t.Fatalf("runLVM: %v", err)
	}
	want := []string{"-p", "my prompt", "-n", "lvremove", "-f", "/dev/vg0/snap"}
	if gotName != "/usr/bin/sudo wrapper" || !reflect.DeepEqual(gotArgs, want) {
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

func TestCreateLV(t *testing.T) {
	if err := lvm.VerifyEscalationCommand(""); err != nil {
		t.Skipf("requires root or --lvm-escalation: %v", err)
	}
	ctx := WithForce(context.Background(), true)
	ctx = WithAllowOverwrite(ctx, true)
	ctx = WithYesIKnow(ctx, true)
	var name string
	var args []string
	cmd := cmdFunc(func(ctx context.Context, n string, a ...string) *exec.Cmd {
		name = n
		args = append([]string(nil), a...)
		return exec.CommandContext(ctx, "true")
	})
	runner := NewDeviceRunner(cmd)
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "vg0"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(dir, "vg0", "new")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatalf("touch: %v", err)
	}
	if err := runner.CreateLV(ctx, path, 1024, ""); err != nil {
		t.Fatalf("CreateLV: %v", err)
	}
	want := []string{"-n", "new", "-L", "1024B", "vg0"}
	if name != "lvcreate" || !reflect.DeepEqual(args, want) {
		t.Fatalf("unexpected command %s %v", name, args)
	}
}

func TestCreateLVRequiresForce(t *testing.T) {
	if err := lvm.VerifyEscalationCommand(""); err != nil {
		t.Skipf("requires root or --lvm-escalation: %v", err)
	}
	cmd := cmdFunc(func(ctx context.Context, n string, a ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "true")
	})
	runner := NewDeviceRunner(cmd)
	if err := runner.CreateLV(context.Background(), "/dev/vg0/new", 1024, ""); err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("expected force error, got %v", err)
	}
}

func TestCreateLVRunLVMError(t *testing.T) {
	if err := lvm.VerifyEscalationCommand(""); err != nil {
		t.Skipf("requires root or --lvm-escalation: %v", err)
	}
	ctx := WithForce(context.Background(), true)
	ctx = WithAllowOverwrite(ctx, true)
	ctx = WithYesIKnow(ctx, true)
	cmd := cmdFunc(func(ctx context.Context, n string, a ...string) *exec.Cmd {
		return exec.CommandContext(ctx, "false")
	})
	runner := NewDeviceRunner(cmd)
	if err := runner.CreateLV(ctx, "/dev/vg0/new", 1024, ""); err == nil {
		t.Fatalf("expected error")
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

func TestConfirmOverwriteTTYLVM(t *testing.T) {
	ctx := WithForce(context.Background(), true)
	r := ttyLVMReader{strings.NewReader("double-confirm\n")}
	if err := confirmOverwrite(ctx, r, io.Discard, func(int) bool { return true }); err != nil {
		t.Fatalf("confirmOverwrite: %v", err)
	}
}

func TestConfirmOverwriteNonTTYLVM(t *testing.T) {
	ctx := WithForce(context.Background(), true)
	r := strings.NewReader("double-confirm\n")
	if err := confirmOverwrite(ctx, r, io.Discard, func(int) bool { return true }); err == nil || !strings.Contains(err.Error(), "--allow-overwrite") {
		t.Fatalf("expected allow-overwrite error, got %v", err)
	}
}

func TestSelectVolumeGroupFromCandidates(t *testing.T) {
	fb := &vgSelectBackend{vgs: map[string]uint64{"vg0": 100, "vg1": 200}}
	r := lvm.NewRunnerWithDeps(nil, func() error { return nil }, nil, fb, "")
	vg, err := r.SelectVolumeGroupForSize(context.Background(), []string{"vg0", "vg1"}, 50)
	if err != nil {
		t.Fatalf("SelectVolumeGroupForSize: %v", err)
	}
	if vg.Name != "vg1" {
		t.Fatalf("expected vg1, got %s", vg.Name)
	}
}

func TestSelectVolumeGroupNoMatchErrDevice(t *testing.T) {
	fb := &vgSelectBackend{vgs: map[string]uint64{"vg0": 100}}
	r := lvm.NewRunnerWithDeps(nil, func() error { return nil }, nil, fb, "")
	if _, err := r.SelectVolumeGroupForSize(context.Background(), []string{"vg1"}, 50); err == nil {
		t.Fatalf("expected error")
	}
}

func TestSnapshotReadOnly(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("root required")
	}
	ctx := WithForce(context.Background(), true)
	ctx = WithAllowOverwrite(ctx, true)
	ctx = WithYesIKnow(ctx, true)
	var cmds []string
	cmd := cmdFunc(func(ctx context.Context, name string, args ...string) *exec.Cmd {
		cmds = append(cmds, name+" "+strings.Join(args, " "))
		return exec.CommandContext(ctx, "true")
	})

	origName := generateSnapshot
	generateSnapshot = func() string { return "snap" }
	defer func() { generateSnapshot = origName }()

	runner := NewDeviceRunner(cmd)
	runner.openLVMOverride = func(ctx context.Context, p string, _ *lvm.FDCache, _ bool, _ bool, _ string, _ *zap.Logger) (*LVMDevice, error) {
		return &LVMDevice{path: p, cleanupPath: p, escalation: "doas -n", logger: zap.NewNop(), runner: runner}, nil
	}

	origEuid := geteuid
	geteuid = func() int { return 1 }
	defer func() { geteuid = origEuid }()

	lvd := &LVMDevice{path: "/dev/vg0/origin", escalation: "doas -n", logger: zap.NewNop(), runner: runner}
	snap, err := lvd.Snapshot(ctx, "1G")
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if err := snap.Cleanup(ctx); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if len(cmds) < 2 || !strings.Contains(cmds[0], "-pr") || !strings.Contains(cmds[1], "-pr") {
		t.Fatalf("commands = %v, expected -pr flags", cmds)
	}
}
