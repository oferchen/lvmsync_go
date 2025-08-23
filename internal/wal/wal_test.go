package wal

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestAppendAndClose(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wal")
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	// seed with dummy header bytes
	if _, err := f.Write(make([]byte, 16)); err != nil {
		t.Fatalf("write header: %v", err)
	}
	if _, err := f.Seek(16, io.SeekStart); err != nil {
		t.Fatalf("seek: %v", err)
	}
	w := New(f, nil)
	if err := w.Append(Range{Start: 1, End: 2}); err != nil {
		t.Fatalf("append: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(data) != 32 { // header + one entry
		t.Fatalf("unexpected size %d", len(data))
	}
}
