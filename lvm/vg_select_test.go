package lvm

import (
	"fmt"
	"testing"
)

type vgBackend struct {
	vgs []VolumeGroup
}

func (f *vgBackend) CreateSnapshot(string, string, string) error { return nil }
func (f *vgBackend) RemoveSnapshot(string) error                 { return nil }
func (f *vgBackend) GetSnapshotUsage(string) (float64, error)    { return 0, nil }
func (f *vgBackend) GetVolumeGroupFreeSpace(name string) (uint64, error) {
	for _, vg := range f.vgs {
		if vg.Name == name {
			return vg.Free, nil
		}
	}
	return 0, fmt.Errorf("unknown vg")
}
func (f *vgBackend) ListVolumeGroups() ([]VolumeGroup, error) { return f.vgs, nil }

func TestSelectVolumeGroupByFreeSpace(t *testing.T) {
	orig := checkPrivs
	checkPrivs = func() error { return nil }
	t.Cleanup(func() { checkPrivs = orig })
	fb := &vgBackend{vgs: []VolumeGroup{{Name: "vg0", Free: 100}, {Name: "vg1", Free: 200}}}
	restore := SetBackend(fb)
	defer restore()

	vg, free, err := SelectVolumeGroupByFreeSpace(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vg != "vg1" || free != 200 {
		t.Fatalf("expected vg1 with 200, got %s with %d", vg, free)
	}

	vg, free, err = SelectVolumeGroupByFreeSpace([]string{"vg0"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vg != "vg0" || free != 100 {
		t.Fatalf("expected vg0 with 100, got %s with %d", vg, free)
	}

	if _, _, err := SelectVolumeGroupByFreeSpace([]string{"vg2"}); err == nil {
		t.Fatalf("expected error for unknown vg")
	}
}
