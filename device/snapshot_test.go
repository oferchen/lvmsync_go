package device

import (
	"context"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"

	"go.uber.org/zap"

	"lvmsync_go/lvm"
)

func TestSnapshotLifecycle(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("root required")
	}
	ctx := WithForce(context.Background(), true)
	ctx = WithAllowOverwrite(ctx, true)
	ctx = WithYesIKnow(ctx, true)

	// File device snapshot is a no-op.
	f, err := os.CreateTemp(t.TempDir(), "file")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	f.Close()
	fd, err := OpenFile(f.Name(), false, zap.NewNop())
	if err != nil {
		t.Fatalf("open file: %v", err)
	}
	fsnap, err := fd.Snapshot(ctx, "")
	if err != nil {
		t.Fatalf("file snapshot: %v", err)
	}
	if fsnap.Path() != fd.Path() {
		t.Fatalf("file snapshot path mismatch")
	}
	if err := fsnap.Cleanup(ctx); err != nil {
		t.Fatalf("file cleanup: %v", err)
	}

	// Raw device snapshot is also a no-op.
	rd := &RawDevice{f: os.NewFile(uintptr(0), "/dev/sda"), logger: zap.NewNop()}
	rsnap, err := rd.Snapshot(ctx, "")
	if err != nil {
		t.Fatalf("raw snapshot: %v", err)
	}
	if rsnap.Path() != rd.Path() {
		t.Fatalf("raw snapshot path mismatch")
	}
	if err := rsnap.Cleanup(ctx); err != nil {
		t.Fatalf("raw cleanup: %v", err)
	}

	// LVM device snapshot creates and removes snapshots via a custom escalation command when non-root.
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
	snap, err := lvd.Snapshot(ctx, "2G")
	if err != nil {
		t.Fatalf("lvm snapshot: %v", err)
	}
	if snap.Path() != "/dev/vg0/snap" {
		t.Fatalf("lvm snapshot path = %s", snap.Path())
	}
	if err := snap.Cleanup(ctx); err != nil {
		t.Fatalf("cleanup: %v", err)
	}

	want := []string{
		"doas -n lvcreate -s -n snap -L 2G -pr /dev/vg0/origin",
		"doas -n lvchange -ay -pr /dev/vg0/snap",
		"doas -n lvremove -f /dev/vg0/snap",
	}
	if !reflect.DeepEqual(cmds, want) {
		t.Fatalf("commands = %v, want %v", cmds, want)
	}
}

func TestSnapshotLVCreateFailure(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("root required")
	}
	ctx := WithForce(context.Background(), true)
	ctx = WithAllowOverwrite(ctx, true)
	ctx = WithYesIKnow(ctx, true)
	var cmds []string
	cmd := cmdFunc(func(ctx context.Context, name string, args ...string) *exec.Cmd {
		cmds = append(cmds, name+" "+strings.Join(args, " "))
		if strings.Contains(strings.Join(args, " "), "lvcreate") {
			return exec.CommandContext(ctx, "false")
		}
		return exec.CommandContext(ctx, "true")
	})

	origEuid := geteuid
	geteuid = func() int { return 1 }
	defer func() { geteuid = origEuid }()

	lvd := &LVMDevice{path: "/dev/vg0/origin", escalation: "doas -n", logger: zap.NewNop(), runner: NewDeviceRunner(cmd)}
	if _, err := lvd.Snapshot(ctx, "1G"); err == nil {
		t.Fatalf("expected error from lvcreate failure")
	}
	if len(cmds) != 1 || !strings.Contains(cmds[0], "lvcreate") {
		t.Fatalf("unexpected commands: %v", cmds)
	}
}
