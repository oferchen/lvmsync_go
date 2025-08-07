package lvm

import (
        "context"
        "testing"

        "lvmsync_go/lvm/cgo"
)

func TestBackend(t *testing.T) {
        mc := newMockCGO()
        mc.usage = 55.5
        mc.vgFree["vg0"] = 1024
        mc.vgFree["vg1"] = 2048
        mc.vgs = []cgo.VolumeGroup{{Name: "vg0", Free: 1024}, {Name: "vg1", Free: 2048}}
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
