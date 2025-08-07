//go:build !linux || !cgo

package cgo

import (
	"errors"
	"testing"
)

func TestStubReturnsErrUnsupported(t *testing.T) {
	dm := New()

	if err := dm.CreateSnapshot("lv", "snap", 1); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("CreateSnapshot error = %v, want ErrUnsupported", err)
	}
	if err := dm.RemoveLV("lv"); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("RemoveLV error = %v, want ErrUnsupported", err)
	}
	if _, err := dm.SnapshotUsage("lv"); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("SnapshotUsage error = %v, want ErrUnsupported", err)
	}
	if _, err := dm.VGFree("vg"); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("VGFree error = %v, want ErrUnsupported", err)
	}
	if _, err := dm.ListVGs(); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("ListVGs error = %v, want ErrUnsupported", err)
	}

	if _, err := Open(); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Open error = %v, want ErrUnsupported", err)
	}
}
