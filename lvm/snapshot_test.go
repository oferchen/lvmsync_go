package lvm

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dpeckett/lvm2"
)

type mockBackend struct{ calls []string }

func (f *mockBackend) CreateSnapshot(ctx context.Context, lvPath, snapName, size string) error {
	f.calls = append(f.calls, fmt.Sprintf("create:%s:%s:%s", lvPath, snapName, size))
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(50 * time.Millisecond):
		return nil
	}
}

func (f *mockBackend) RemoveSnapshot(ctx context.Context, path string) error {
	f.calls = append(f.calls, fmt.Sprintf("remove:%s", path))
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(50 * time.Millisecond):
		return nil
	}
}

func (f *mockBackend) GetSnapshotUsage(context.Context, string) (float64, error) {
	return 0, nil
}
func (f *mockBackend) GetVolumeGroupFreeSpace(context.Context, string) (uint64, error) {
	return 0, nil
}
func (f *mockBackend) ListVolumeGroups(context.Context, []string) ([]VolumeGroup, error) {
	return nil, nil
}

func init() {
	SetEscalationCommand("")
}

func TestCreateAndRemoveSnapshot(t *testing.T) {
	orig := checkPrivs
	checkPrivs = func() error { return nil }
	t.Cleanup(func() { checkPrivs = orig })

	fb := &mockBackend{}
	restore := SetBackend(fb)
	t.Cleanup(restore)

	lvPath := "/dev/vg0/origin"
	snapName := "snap"
	size := "1G"

	if err := CreateSnapshot(context.Background(), lvPath, snapName, size); err != nil {
		t.Fatalf("CreateSnapshot failed: %v", err)
	}
	snapPath := "/dev/vg0/" + snapName
	if err := RemoveSnapshot(context.Background(), snapPath); err != nil {
		t.Fatalf("RemoveSnapshot failed: %v", err)
	}

	if len(fb.calls) != 2 {
		t.Fatalf("expected 2 calls, got %d", len(fb.calls))
	}
	if fb.calls[0] != fmt.Sprintf("create:%s:%s:%s", lvPath, snapName, size) {
		t.Fatalf("unexpected create call %q", fb.calls[0])
	}
	if fb.calls[1] != fmt.Sprintf("remove:%s", snapPath) {
		t.Fatalf("unexpected remove call %q", fb.calls[1])
	}
}

func TestCreateSnapshotPrivilegeError(t *testing.T) {
	orig := checkPrivs
	errPriv := errors.New("privileges required")
	checkPrivs = func() error { return errPriv }
	t.Cleanup(func() { checkPrivs = orig })

	err := CreateSnapshot(context.Background(), "/dev/vg0/origin", "snap", "1G")
	if !errors.Is(err, errPriv) {
		t.Fatalf("expected privilege error, got %v", err)
	}
}

func TestCreateSnapshotContextCancel(t *testing.T) {
	orig := checkPrivs
	checkPrivs = func() error { return nil }
	t.Cleanup(func() { checkPrivs = orig })

	fb := &mockBackend{}
	restore := SetBackend(fb)
	t.Cleanup(restore)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	lvPath := "/dev/vg0/origin"
	snapName := "snap"
	size := "1G"

	err := CreateSnapshot(ctx, lvPath, snapName, size)
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context deadline exceeded, got %v", err)
	}

	// backend should have recorded attempt before ctx cancelled
	if len(fb.calls) == 0 || fb.calls[0] != fmt.Sprintf("create:%s:%s:%s", lvPath, snapName, size) {
		t.Fatalf("backend call not recorded")
	}
}

func TestLVM2BackendCreateSnapshotUsesVGAndLV(t *testing.T) {
	tmpDir := t.TempDir()
	argsFile := filepath.Join(tmpDir, "args")
	script := filepath.Join(tmpDir, "lvm.sh")
	content := fmt.Sprintf("#!/bin/sh\nprintf '%%s ' \"$@\" > %s\n", argsFile)
	if err := os.WriteFile(script, []byte(content), 0700); err != nil {
		t.Fatalf("failed to create script: %v", err)
	}

	b := &lvm2Backend{client: lvm2.NewClient(lvm2.WithLVM(script))}

	if err := b.CreateSnapshot(context.Background(), "/dev/vg0/origin", "snap", "1G"); err != nil {
		t.Fatalf("CreateSnapshot failed: %v", err)
	}

	data, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("failed to read args file: %v", err)
	}
	args := strings.Fields(string(data))
	if len(args) < 2 {
		t.Fatalf("insufficient arguments: %v", args)
	}
	vg, lv := args[len(args)-2], args[len(args)-1]
	if vg != "vg0" || lv != "origin" {
		t.Fatalf("unexpected VG/LV args: %v", args[len(args)-2:])
	}
}
