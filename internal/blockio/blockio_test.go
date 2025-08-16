package blockio

import (
	"bytes"
	"io"
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

type countingReader struct {
	data  []byte
	off   int
	calls int
}

func newCountingReader(size int) *countingReader {
	return &countingReader{data: bytes.Repeat([]byte{0x5}, size)}
}

func (r *countingReader) Read(p []byte) (int, error) {
	r.calls++
	if r.off >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.off:])
	r.off += n
	return n, nil
}

type countingWriter struct {
	w     io.Writer
	calls int
}

func (w *countingWriter) Write(p []byte) (int, error) {
	w.calls++
	return w.w.Write(p)
}

func TestReadFromSyscallReduction(t *testing.T) {
	const size = 1 << 20
	base := newCountingReader(size)
	if _, err := io.Copy(io.Discard, base); err != nil {
		t.Fatalf("baseline copy: %v", err)
	}
	if base.calls <= 1 {
		t.Fatalf("expected baseline reads >1, got %d", base.calls)
	}

	cr := newCountingReader(size)
	tmp, err := os.CreateTemp(t.TempDir(), "blk")
	if err != nil {
		t.Fatalf("tempfile: %v", err)
	}
	f, err := Open(tmp.Name(), false, false)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()
	n, err := io.Copy(f, cr)
	if err != nil {
		t.Fatalf("copy: %v", err)
	}
	if n != size {
		t.Fatalf("copied %d want %d", n, size)
	}
	if cr.calls >= base.calls {
		t.Fatalf("expected fewer reads: %d >= %d", cr.calls, base.calls)
	}
}

func TestWriteToSyscallReduction(t *testing.T) {
	const size = 1 << 20
	data := bytes.Repeat([]byte{0x6}, size)
	tmp, err := os.CreateTemp(t.TempDir(), "blk")
	if err != nil {
		t.Fatalf("tempfile: %v", err)
	}
	if err := os.WriteFile(tmp.Name(), data, 0600); err != nil {
		t.Fatalf("writefile: %v", err)
	}

	fBase, err := Open(tmp.Name(), false, false)
	if err != nil {
		t.Fatalf("open baseline: %v", err)
	}
	cwBase := &countingWriter{w: io.Discard}
	type noWriteTo struct{ io.Reader }
	if _, err := io.Copy(cwBase, noWriteTo{fBase}); err != nil {
		t.Fatalf("baseline copy: %v", err)
	}
	if cwBase.calls <= 1 {
		t.Fatalf("expected baseline writes >1, got %d", cwBase.calls)
	}
	fBase.Close()

	f, err := Open(tmp.Name(), false, false)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()
	cw := &countingWriter{w: io.Discard}
	if _, err := io.Copy(cw, f); err != nil {
		t.Fatalf("copy: %v", err)
	}
	if cw.calls >= cwBase.calls {
		t.Fatalf("expected fewer writes: %d >= %d", cw.calls, cwBase.calls)
	}
}
