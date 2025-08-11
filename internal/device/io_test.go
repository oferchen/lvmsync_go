package device

import (
	"errors"
	"os"
	"testing"

	"golang.org/x/sys/unix"
)

func TestAlign(t *testing.T) {
	if AlignDown(4097, 4096) != 4096 {
		t.Fatalf("algin down")
	}
	if AlignUp(4097, 4096) != 8192 {
		t.Fatalf("align up")
	}
}

func TestBlockSizes(t *testing.T) {
	f, err := os.CreateTemp("", "blk")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	defer f.Close()
	l, p, err := BlockSizes(f)
	if err != nil {
		t.Fatal(err)
	}
	if l <= 0 || p <= 0 {
		t.Fatalf("invalid sizes")
	}
}

func TestOpenDirect(t *testing.T) {
	f, err := OpenDirect("/dev/zero")
	if err != nil {
		if errors.Is(err, unix.EINVAL) || errors.Is(err, unix.EPERM) {
			t.Skip("O_DIRECT unsupported")
		}
		t.Fatalf("open direct: %v", err)
	}
	f.Close()
}

func TestFileWriter(t *testing.T) {
	f, err := os.CreateTemp("", "w")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	w := &FileWriter{f}
	if _, err := w.WriteAt([]byte("hi"), 0); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := w.Sync(); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if err := w.Discard(0, 2); err != nil {
		t.Fatalf("discard: %v", err)
	}
	f.Close()
}
