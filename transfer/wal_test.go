package transfer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWALMismatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wal")
	w, _, err := OpenWAL(path, 100, "dev", 1)
	if err != nil {
		t.Fatalf("open wal: %v", err)
	}
	w.Close()
	if _, _, err := OpenWAL(path, 101, "dev", 1); err == nil {
		t.Fatalf("expected size mismatch error")
	}
	if _, _, err := OpenWAL(path, 100, "dev2", 1); err == nil {
		t.Fatalf("expected device mismatch error")
	}
	if _, _, err := OpenWAL(path, 100, "dev", 2); err == nil {
		t.Fatalf("expected epoch mismatch error")
	}
}

func TestWALDurability(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wal")
	w, _, err := OpenWAL(path, 100, "dev", 1)
	if err != nil {
		t.Fatalf("open wal: %v", err)
	}
	if err := w.Append(Range{Start: 0, End: 10}); err != nil {
		t.Fatalf("append: %v", err)
	}
	w.Close()
	w, ranges, err := OpenWAL(path, 100, "dev", 1)
	if err != nil {
		t.Fatalf("reopen wal: %v", err)
	}
	if len(ranges) != 1 || ranges[0].Start != 0 || ranges[0].End != 10 {
		t.Fatalf("unexpected ranges %#v", ranges)
	}
	w.Close()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("stat wal: %v", err)
	}
}
