package blockio

import (
	"bytes"
	"os"
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
