package device

import (
	"path/filepath"
	"testing"
)

func TestWALIdentityMismatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wal")
	id := DeviceIdentity{SizeBytes: 100, KernelUUID: "dev", GPTUUID: "gpt"}
	w, err := OpenWAL(path, id)
	if err != nil {
		t.Fatalf("open wal: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	badID := DeviceIdentity{SizeBytes: 101, KernelUUID: "dev", GPTUUID: "gpt"}
	if _, err := OpenWAL(path, badID); err == nil {
		t.Fatalf("expected size mismatch error")
	}
	badID = DeviceIdentity{SizeBytes: 100, KernelUUID: "dev2", GPTUUID: "gpt"}
	if _, err := OpenWAL(path, badID); err == nil {
		t.Fatalf("expected uuid mismatch error")
	}
}

func TestWALRecovery(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wal")
	id := DeviceIdentity{SizeBytes: 100, KernelUUID: "dev", GPTUUID: "gpt"}
	w, err := OpenWAL(path, id)
	if err != nil {
		t.Fatalf("open wal: %v", err)
	}
	if err := w.Append(Range{Start: 0, End: 10}); err != nil {
		t.Fatalf("append: %v", err)
	}
	w.Close()
	w, err = OpenWAL(path, id)
	if err != nil {
		t.Fatalf("reopen wal: %v", err)
	}
	rs := w.Ranges()
	if len(rs) != 1 || rs[0].Start != 0 || rs[0].End != 10 {
		t.Fatalf("unexpected ranges %#v", rs)
	}
	w.Close()
}
