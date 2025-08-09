package blocksize

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestReadPreferredSize(t *testing.T) {
	dir := t.TempDir()
	// write only physical_block_size
	if err := os.WriteFile(filepath.Join(dir, "physical_block_size"), []byte("8192\n"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	size, err := readPreferredSize(dir)
	if err != nil {
		t.Fatalf("readPreferredSize error: %v", err)
	}
	if size != 8192 {
		t.Fatalf("expected 8192, got %d", size)
	}
}

func TestReadPreferredSizeOrder(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "physical_block_size"), []byte("4096\n"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "optimal_io_size"), []byte("32768\n"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	size, err := readPreferredSize(dir)
	if err != nil {
		t.Fatalf("readPreferredSize error: %v", err)
	}
	if size != 32768 {
		t.Fatalf("expected 32768, got %d", size)
	}
}

func TestReadPreferredSizeFallback(t *testing.T) {
	dir := t.TempDir()
	size, err := readPreferredSize(dir)
	if err != nil {
		t.Fatalf("readPreferredSize error: %v", err)
	}
	if size != 4096 {
		t.Fatalf("expected 4096, got %d", size)
	}
}

func TestDetect(t *testing.T) {
	dir := t.TempDir()
	devPath := filepath.Join(dir, "ram0")
	dev := unix.Mkdev(1, 0)
	if err := unix.Mknod(devPath, unix.S_IFBLK|0600, int(dev)); err != nil {
		t.Skipf("mknod: %v", err)
	}
	size, err := Detect(devPath)
	if err != nil {
		t.Fatalf("Detect error: %v", err)
	}
	if size != 4096 {
		t.Fatalf("expected 4096, got %d", size)
	}
}

func TestDetectFallback(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file")
	if err := os.WriteFile(file, nil, 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	size, err := Detect(file)
	if err != nil {
		t.Fatalf("Detect error: %v", err)
	}
	if size != 4096 {
		t.Fatalf("expected 4096, got %d", size)
	}
}
