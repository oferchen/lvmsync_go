package device

import (
	"os"
	"testing"
)

func TestOpenFile(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "filedev")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	if _, err := f.Write(make([]byte, 4096)); err != nil {
		t.Fatalf("write: %v", err)
	}
	path := f.Name()
	f.Close()

	d, err := OpenFile(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer d.Close()

	if d.SizeBytes() != 4096 {
		t.Fatalf("expected size 4096, got %d", d.SizeBytes())
	}
	if d.BlockSize() == 0 {
		t.Fatalf("expected non-zero block size")
	}
}

func TestOpenFileRejectsNonRegular(t *testing.T) {
	dir := t.TempDir()
	if _, err := OpenFile(dir); err == nil {
		t.Fatalf("expected error for directory")
	}
}
