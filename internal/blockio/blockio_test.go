package blockio

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteAligned(t *testing.T) {
	tmp, err := os.CreateTemp(t.TempDir(), "blk")
	if err != nil {
		t.Fatalf("tempfile: %v", err)
	}
	f, err := Open(tmp.Name(), false, false)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()
	data := bytes.Repeat([]byte{0x1}, f.Logical()*2)
	if _, err := f.Write(data); err != nil {
		t.Fatalf("write: %v", err)
	}
	fi, _ := tmp.Stat()
	if int(fi.Size()) != len(data) {
		t.Fatalf("size %d want %d", fi.Size(), len(data))
	}
}

func TestWriteMisalignedFallback(t *testing.T) {
	tmp, err := os.CreateTemp(t.TempDir(), "blk")
	if err != nil {
		t.Fatalf("tempfile: %v", err)
	}
	f, err := Open(tmp.Name(), true, false)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()
	f.direct = true // simulate O_DIRECT
	data := bytes.Repeat([]byte{0x2}, f.Logical()-1)
	if _, err := f.Write(data); err != nil {
		t.Fatalf("write: %v", err)
	}
	if f.Direct() {
		t.Fatalf("expected fallback to buffered IO")
	}
	fi, _ := tmp.Stat()
	if int(fi.Size()) != len(data) {
		t.Fatalf("size %d want %d", fi.Size(), len(data))
	}
}

func TestWriteMisalignedStrict(t *testing.T) {
	tmp, err := os.CreateTemp(t.TempDir(), "blk")
	if err != nil {
		t.Fatalf("tempfile: %v", err)
	}
	f, err := Open(tmp.Name(), true, true)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()
	f.direct = true
	data := bytes.Repeat([]byte{0x3}, f.Logical()-1)
	if _, err := f.Write(data); err == nil {
		t.Fatalf("expected error for misaligned write")
	}
	fi, _ := tmp.Stat()
	if fi.Size() != 0 {
		t.Fatalf("expected no data written")
	}
}

func TestWritePhysicalMisaligned(t *testing.T) {
	tmp, err := os.CreateTemp(t.TempDir(), "blk")
	if err != nil {
		t.Fatalf("tempfile: %v", err)
	}
	f, err := Open(tmp.Name(), true, false)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()
	f.direct = true
	f.physical = f.logical * 2
	data := bytes.Repeat([]byte{0x4}, f.Logical())
	if _, err := f.Write(data); err != nil {
		t.Fatalf("write: %v", err)
	}
	if f.Direct() {
		t.Fatalf("expected fallback for physical misalignment")
	}
}

func TestOpenNonexistentPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing")
	if _, err := Open(path, false, false); err == nil {
		t.Fatalf("expected error for nonexistent path")
	}
}

func TestOpenBlockDevice(t *testing.T) {
	entries, err := os.ReadDir("/dev")
	if err != nil {
		t.Fatalf("readdir /dev: %v", err)
	}
	var dev string
	for _, e := range entries {
		p := filepath.Join("/dev", e.Name())
		fi, err := os.Stat(p)
		if err != nil {
			continue
		}
		mode := fi.Mode()
		if mode&os.ModeDevice != 0 && mode&os.ModeCharDevice == 0 && mode.Perm()&0600 == 0600 {
			dev = p
			break
		}
	}
	if dev == "" {
		t.Skip("no block device available")
	}
	f, err := Open(dev, false, false)
	if err != nil {
		t.Fatalf("open block device %s: %v", dev, err)
	}
	f.Close()
}
