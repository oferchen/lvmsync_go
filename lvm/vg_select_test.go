package lvm

import (
	"fmt"
	"testing"
)

func TestSelectVolumeGroupByFreeSpace(t *testing.T) {
	orig := checkPrivs
	checkPrivs = func() error { return nil }
	t.Cleanup(func() { checkPrivs = orig })
	restore := SetRunLVMCommand(func(name string, args ...string) ([]byte, error) {
		if name != "vgs" {
			return nil, fmt.Errorf("unexpected command %s", name)
		}
		return []byte("vg0:100B\nvg1:200B\n"), nil
	})
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
