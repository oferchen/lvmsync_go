package blockio

import (
	"bytes"
	"os"
	"testing"
)

func TestReaderFromAligned(t *testing.T) {
	tmp, err := os.CreateTemp(t.TempDir(), "blk")
	if err != nil {
		t.Fatalf("tempfile: %v", err)
	}
	f, err := Open(tmp.Name(), false)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()
	data := bytes.Repeat([]byte{0x1}, f.Logical()*2)
	n, err := f.ReaderFrom(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("readerfrom: %v", err)
	}
	if int(n) != len(data) {
		t.Fatalf("wrote %d want %d", n, len(data))
	}
	fi, _ := tmp.Stat()
	if int(fi.Size()) != len(data) {
		t.Fatalf("size %d want %d", fi.Size(), len(data))
	}
}

func TestReaderFromMisalignedFallback(t *testing.T) {
	tmp, err := os.CreateTemp(t.TempDir(), "blk")
	if err != nil {
		t.Fatalf("tempfile: %v", err)
	}
	f, err := Open(tmp.Name(), false)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()
	f.direct = true // simulate O_DIRECT
	data := bytes.Repeat([]byte{0x2}, f.Logical()-1)
	if _, err := f.ReaderFrom(bytes.NewReader(data)); err != nil {
		t.Fatalf("readerfrom: %v", err)
	}
	if f.Direct() {
		t.Fatalf("expected fallback to buffered IO")
	}
	fi, _ := tmp.Stat()
	if int(fi.Size()) != len(data) {
		t.Fatalf("size %d want %d", fi.Size(), len(data))
	}
}

func TestReaderFromMisalignedStrict(t *testing.T) {
	tmp, err := os.CreateTemp(t.TempDir(), "blk")
	if err != nil {
		t.Fatalf("tempfile: %v", err)
	}
	f, err := Open(tmp.Name(), true)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()
	f.direct = true
	data := bytes.Repeat([]byte{0x3}, f.Logical()-1)
	if _, err := f.ReaderFrom(bytes.NewReader(data)); err == nil {
		t.Fatalf("expected error for misaligned write")
	}
	fi, _ := tmp.Stat()
	if fi.Size() != 0 {
		t.Fatalf("expected no data written")
	}
}

func TestReaderFromSync(t *testing.T) {
	tmp, err := os.CreateTemp(t.TempDir(), "blk")
	if err != nil {
		t.Fatalf("tempfile: %v", err)
	}
	f, err := Open(tmp.Name(), false)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()
	called := false
	f.syncFunc = func() error { called = true; return nil }
	data := bytes.Repeat([]byte{0x4}, f.Logical())
	if _, err := f.ReaderFrom(bytes.NewReader(data)); err != nil {
		t.Fatalf("readerfrom: %v", err)
	}
	if !called {
		t.Fatalf("expected syncFunc to be called")
	}
}

func TestWriterTo(t *testing.T) {
	tmp, err := os.CreateTemp(t.TempDir(), "blk")
	if err != nil {
		t.Fatalf("tempfile: %v", err)
	}
	f, err := Open(tmp.Name(), false)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()
	data := bytes.Repeat([]byte{0x5}, f.Logical()*2)
	if _, err := f.ReaderFrom(bytes.NewReader(data)); err != nil {
		t.Fatalf("prep write: %v", err)
	}
	var buf bytes.Buffer
	if _, err := f.f.Seek(0, 0); err != nil {
		t.Fatalf("seek: %v", err)
	}
	n, err := f.WriterTo(&buf)
	if err != nil {
		t.Fatalf("writerto: %v", err)
	}
	if int(n) != len(data) {
		t.Fatalf("read %d want %d", n, len(data))
	}
	if !bytes.Equal(buf.Bytes(), data) {
		t.Fatalf("data mismatch")
	}
}
