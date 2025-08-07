//go:build !linux || !cgo || !lvm2cmd

package cgo

import (
	"errors"
	"testing"
)

func TestStubReturnsErrUnsupported(t *testing.T) {
	lvm := New()

	if err := lvm.CreateSnapshot("lv", "snap", 1); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("CreateSnapshot error = %v, want ErrUnsupported", err)
	}
	if err := lvm.RemoveLV("lv"); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("RemoveLV error = %v, want ErrUnsupported", err)
	}
	if _, err := lvm.SnapshotUsage("lv"); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("SnapshotUsage error = %v, want ErrUnsupported", err)
	}
	if _, err := lvm.VGFree("vg"); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("VGFree error = %v, want ErrUnsupported", err)
	}
	if _, err := lvm.ListVGs(); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("ListVGs error = %v, want ErrUnsupported", err)
	}
}
