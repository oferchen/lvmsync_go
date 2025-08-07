package lvm

import (
	"context"
	"fmt"
	"testing"

	"lvmsync_go/lvm/cgo"
)

type mockCGO struct{ calls []string }

func (m *mockCGO) CreateSnapshot(lvPath, snapName string, sizeBytes uint64) error {
	m.calls = append(m.calls, fmt.Sprintf("create:%s:%s:%d", lvPath, snapName, sizeBytes))
	return nil
}
func (m *mockCGO) RemoveLV(lvPath string) error {
	m.calls = append(m.calls, fmt.Sprintf("remove:%s", lvPath))
	return nil
}
func (m *mockCGO) SnapshotUsage(string) (float64, error) { return 55.5, nil }
func (m *mockCGO) VGFree(vgName string) (uint64, error) {
	if vgName == "vg0" {
		return 1024, nil
	}
	if vgName == "vg1" {
		return 2048, nil
	}
	return 0, fmt.Errorf("unknown vg")
}
func (m *mockCGO) ListVGs() ([]cgo.VolumeGroup, error) {
	return []cgo.VolumeGroup{{Name: "vg0", Free: 1024}, {Name: "vg1", Free: 2048}}, nil
}

func TestBackend(t *testing.T) {
	mc := &mockCGO{}
	b := newBackendWithCGO(mc)
	ctx := context.Background()

	if err := b.CreateSnapshot(ctx, "/dev/vg0/origin", "snap", "1G"); err != nil {
		t.Fatalf("CreateSnapshot failed: %v", err)
	}
	if err := b.RemoveSnapshot(ctx, "/dev/vg0/snap"); err != nil {
		t.Fatalf("RemoveSnapshot failed: %v", err)
	}
	usage, err := b.GetSnapshotUsage(ctx, "/dev/vg0/snap")
	if err != nil || usage != 55.5 {
		t.Fatalf("unexpected usage %.1f err %v", usage, err)
	}
	free, err := b.GetVolumeGroupFreeSpace(ctx, "vg0")
	if err != nil || free != 1024 {
		t.Fatalf("unexpected free %d err %v", free, err)
	}
	vgs, err := b.ListVolumeGroups(ctx, nil)
	if err != nil || len(vgs) != 2 {
		t.Fatalf("ListVolumeGroups failed: %v", err)
	}
	vgs, err = b.ListVolumeGroups(ctx, []string{"vg1"})
	if err != nil || len(vgs) != 1 || vgs[0].Name != "vg1" || vgs[0].Free != 2048 {
		t.Fatalf("unexpected filtered result: %#v err %v", vgs, err)
	}
	if len(mc.calls) < 2 || mc.calls[0] == "" {
		t.Fatalf("expected calls to underlying cgo wrapper")
	}
}
