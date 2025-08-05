package lvm

import (
	"errors"
	"fmt"
	"testing"
)

type mockBackend struct{ calls []string }

func (f *mockBackend) CreateSnapshot(lvPath, snapName, size string) error {
	f.calls = append(f.calls, fmt.Sprintf("create:%s:%s:%s", lvPath, snapName, size))
	return nil
}

func (f *mockBackend) RemoveSnapshot(path string) error {
	f.calls = append(f.calls, fmt.Sprintf("remove:%s", path))
	return nil
}

func (f *mockBackend) GetSnapshotUsage(string) (float64, error)       { return 0, nil }
func (f *mockBackend) GetVolumeGroupFreeSpace(string) (uint64, error) { return 0, nil }
func (f *mockBackend) ListVolumeGroups() ([]VolumeGroup, error)       { return nil, nil }

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

	if err := CreateSnapshot(lvPath, snapName, size); err != nil {
		t.Fatalf("CreateSnapshot failed: %v", err)
	}
	snapPath := "/dev/vg0/" + snapName
	if err := RemoveSnapshot(snapPath); err != nil {
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

	err := CreateSnapshot("/dev/vg0/origin", "snap", "1G")
	if !errors.Is(err, errPriv) {
		t.Fatalf("expected privilege error, got %v", err)
	}
}
