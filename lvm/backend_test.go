package lvm

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oferchen/lvmsync_go/lvm/cgo"
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
	wantCreate := fmt.Sprintf("create:%s:%s:%d:ro", "/dev/vg0/origin", "snap", uint64(1000000000))
	wantRemove := "remove:/dev/vg0/snap"
	if len(mc.calls) != 2 || mc.calls[0] != wantCreate || mc.calls[1] != wantRemove {
		t.Fatalf("calls = %v, want [%s %s]", mc.calls, wantCreate, wantRemove)
	}
}

func TestParseLVPath(t *testing.T) {
	root := t.TempDir()
	dev := filepath.Join(root, "dev")
	mapper := filepath.Join(dev, "mapper")
	if err := os.MkdirAll(mapper, 0o755); err != nil {
		t.Fatalf("mkdir mapper: %v", err)
	}

	// device representing /dev/mapper/vg-lv
	device := filepath.Join(mapper, "vg-lv")
	if err := os.WriteFile(device, nil, 0o644); err != nil {
		t.Fatalf("create device: %v", err)
	}

	// /dev/vg/lv -> /dev/mapper/vg-lv
	vgDir := filepath.Join(dev, "vg")
	if err := os.MkdirAll(vgDir, 0o755); err != nil {
		t.Fatalf("mkdir vg: %v", err)
	}
	devPath := filepath.Join(vgDir, "lv")
	if err := os.Symlink(device, devPath); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	// create plain /dev/vg2/lv2 without mapper symlink
	vgPlain := filepath.Join(dev, "vg2")
	if err := os.MkdirAll(vgPlain, 0o755); err != nil {
		t.Fatalf("mkdir vg2: %v", err)
	}
	plain := filepath.Join(vgPlain, "lv2")
	if err := os.WriteFile(plain, nil, 0o644); err != nil {
		t.Fatalf("create plain: %v", err)
	}

	// alias symlink outside of vg/lv naming
	aliasDir := filepath.Join(dev, "by-id")
	if err := os.MkdirAll(aliasDir, 0o755); err != nil {
		t.Fatalf("mkdir alias: %v", err)
	}
	aliasPath := filepath.Join(aliasDir, "alias")
	if err := os.Symlink(device, aliasPath); err != nil {
		t.Fatalf("symlink alias: %v", err)
	}

	replacer := strings.NewReplacer("-", "--")
	encoded := filepath.Join(mapper, replacer.Replace("vg-hy")+"-"+replacer.Replace("lv-hy"))
	if err := os.WriteFile(encoded, nil, 0o644); err != nil {
		t.Fatalf("create encoded: %v", err)
	}

	tests := []struct {
		path string
		vg   string
		lv   string
	}{
		{devPath, "vg", "lv"},
		{plain, "vg2", "lv2"},
		{device, "vg", "lv"},
		{aliasPath, "vg", "lv"},
		{encoded, "vg-hy", "lv-hy"},
	}

	for _, tt := range tests {
		vg, lv, err := ParseLVPath(tt.path)
		if err != nil {
			t.Fatalf("ParseLVPath(%s) error: %v", tt.path, err)
		}
		if vg != tt.vg || lv != tt.lv {
			t.Fatalf("ParseLVPath(%s) = %s/%s, want %s/%s", tt.path, vg, lv, tt.vg, tt.lv)
		}
	}
}
