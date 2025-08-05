package lvm

import (
	"context"
	"fmt"
	"reflect"
	"testing"
)

type vgBackend struct {
	vgs      []VolumeGroup
	lastArgs []string
}

func (f *vgBackend) CreateSnapshot(context.Context, string, string, string) error { return nil }
func (f *vgBackend) RemoveSnapshot(context.Context, string) error                 { return nil }
func (f *vgBackend) GetSnapshotUsage(context.Context, string) (float64, error)    { return 0, nil }
func (f *vgBackend) GetVolumeGroupFreeSpace(ctx context.Context, name string) (uint64, error) {
	for _, vg := range f.vgs {
		if vg.Name == name {
			return vg.Free, nil
		}
	}
	return 0, fmt.Errorf("unknown vg")
}
func (f *vgBackend) ListVolumeGroups(_ context.Context, candidates []string) ([]VolumeGroup, error) {
	f.lastArgs = candidates
	if len(candidates) == 0 {
		return f.vgs, nil
	}
	res := []VolumeGroup{}
	for _, name := range candidates {
		for _, vg := range f.vgs {
			if vg.Name == name {
				res = append(res, vg)
				break
			}
		}
	}
	return res, nil
}

func TestSelectVolumeGroup(t *testing.T) {
	orig := checkPrivs
	checkPrivs = func() error { return nil }
	t.Cleanup(func() { checkPrivs = orig })
	fb := &vgBackend{vgs: []VolumeGroup{{Name: "vg0", Free: 100}, {Name: "vg1", Free: 200}}}
	restore := SetBackend(fb)
	defer restore()

	vg, err := SelectVolumeGroup(context.Background(), nil, ByFreeSpace)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vg.Name != "vg1" || vg.Free != 200 {
		t.Fatalf("expected vg1 with 200, got %s with %d", vg.Name, vg.Free)
	}

	vg, err = SelectVolumeGroup(context.Background(), []string{"vg0"}, ByFreeSpace)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vg.Name != "vg0" || vg.Free != 100 {
		t.Fatalf("expected vg0 with 100, got %s with %d", vg.Name, vg.Free)
	}

	if _, err := SelectVolumeGroup(context.Background(), []string{"vg2"}, ByFreeSpace); err == nil {
		t.Fatalf("expected error for unknown vg")
	}
}

func TestSelectVolumeGroupQueriesCandidates(t *testing.T) {
	orig := checkPrivs
	checkPrivs = func() error { return nil }
	t.Cleanup(func() { checkPrivs = orig })
	fb := &vgBackend{vgs: []VolumeGroup{{Name: "vg0", Free: 100}, {Name: "vg1", Free: 200}}}
	restore := SetBackend(fb)
	defer restore()

	if _, err := SelectVolumeGroup(context.Background(), []string{"vg0"}, ByFreeSpace); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(fb.lastArgs, []string{"vg0"}) {
		t.Fatalf("expected backend queried with [vg0], got %v", fb.lastArgs)
	}
}

func TestSelectVolumeGroupCustomSelector(t *testing.T) {
	orig := checkPrivs
	checkPrivs = func() error { return nil }
	t.Cleanup(func() { checkPrivs = orig })
	fb := &vgBackend{vgs: []VolumeGroup{{Name: "vg0", Free: 100}, {Name: "vg1", Free: 200}}}
	restore := SetBackend(fb)
	defer restore()

	selector := func(vgs []VolumeGroup) (VolumeGroup, error) {
		for _, vg := range vgs {
			if vg.Name == "vg0" {
				return vg, nil
			}
		}
		return VolumeGroup{}, fmt.Errorf("no suitable volume group")
	}

	vg, err := SelectVolumeGroup(context.Background(), nil, selector)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vg.Name != "vg0" {
		t.Fatalf("expected vg0, got %s", vg.Name)
	}
}

func TestSelectVolumeGroupForSize(t *testing.T) {
	orig := checkPrivs
	checkPrivs = func() error { return nil }
	t.Cleanup(func() { checkPrivs = orig })
	fb := &vgBackend{vgs: []VolumeGroup{{Name: "vg0", Free: 100}, {Name: "vg1", Free: 200}}}
	restore := SetBackend(fb)
	defer restore()

	vg, err := SelectVolumeGroupForSize(context.Background(), nil, 150)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vg.Name != "vg1" {
		t.Fatalf("expected vg1, got %s", vg.Name)
	}

	if _, err := SelectVolumeGroupForSize(context.Background(), nil, 250); err == nil {
		t.Fatalf("expected error when no vg has enough space")
	}
}
