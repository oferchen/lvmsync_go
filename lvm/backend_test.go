package lvm

import (
	"context"
	"testing"
)

func TestGolvmBackend(t *testing.T) {
	b := newLVMBackend()
	ctx := context.Background()

	if err := b.CreateSnapshot(ctx, "/dev/vg0/origin", "snap", "1G"); err != nil {
		t.Fatalf("CreateSnapshot failed: %v", err)
	}

	if err := b.RemoveSnapshot(ctx, "/dev/vg0/snap"); err != nil {
		t.Fatalf("RemoveSnapshot failed: %v", err)
	}

	usage, err := b.GetSnapshotUsage(ctx, "/dev/vg0/snap")
	if err != nil {
		t.Fatalf("GetSnapshotUsage failed: %v", err)
	}
	if usage != 55.5 {
		t.Fatalf("unexpected usage %.1f", usage)
	}

	free, err := b.GetVolumeGroupFreeSpace(ctx, "vg0")
	if err != nil {
		t.Fatalf("GetVolumeGroupFreeSpace failed: %v", err)
	}
	if free != 1024 {
		t.Fatalf("unexpected free %d", free)
	}

	vgs, err := b.ListVolumeGroups(ctx, nil)
	if err != nil {
		t.Fatalf("ListVolumeGroups failed: %v", err)
	}
	if len(vgs) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(vgs))
	}

	vgs, err = b.ListVolumeGroups(ctx, []string{"vg1"})
	if err != nil {
		t.Fatalf("ListVolumeGroups with filter failed: %v", err)
	}
	if len(vgs) != 1 || vgs[0].Name != "vg1" || vgs[0].Free != 2048 {
		t.Fatalf("unexpected filtered result: %#v", vgs)
	}
}
