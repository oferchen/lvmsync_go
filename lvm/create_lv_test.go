package lvm

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"
)

type createLVBackend struct{ err error }

func (m *createLVBackend) CreateSnapshot(context.Context, string, string, string) error { return nil }
func (m *createLVBackend) RemoveSnapshot(context.Context, string) error                 { return nil }
func (m *createLVBackend) GetSnapshotUsage(context.Context, string) (float64, error)    { return 0, nil }
func (m *createLVBackend) GetVolumeGroupFreeSpace(context.Context, string) (uint64, error) {
	return 0, nil
}
func (m *createLVBackend) ListVolumeGroups(context.Context, []string) ([]VolumeGroup, error) {
	return nil, nil
}
func (m *createLVBackend) CreateLogicalVolume(context.Context, string, string, uint64) error {
	return m.err
}

func TestCreateLogicalVolume(t *testing.T) {
	restorePriv := SetPrivilegeChecker(func() error { return nil })
	t.Cleanup(restorePriv)
	b := &createLVBackend{}
	restore := SetBackend(b)
	t.Cleanup(restore)
	if err := CreateLogicalVolume(context.Background(), "vg", "lv", 1024, zap.NewNop()); err != nil {
		t.Fatalf("CreateLogicalVolume error: %v", err)
	}
}

func TestCreateLogicalVolumeError(t *testing.T) {
	restorePriv := SetPrivilegeChecker(func() error { return nil })
	t.Cleanup(restorePriv)
	b := &createLVBackend{err: errors.New("fail")}
	restore := SetBackend(b)
	t.Cleanup(restore)
	if err := CreateLogicalVolume(context.Background(), "vg", "lv", 1024, zap.NewNop()); err == nil {
		t.Fatal("expected error")
	}
}
