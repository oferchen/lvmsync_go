package transfer

import (
	"encoding/binary"
	"io"
	"io/fs"
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

func TestWALRecovery(t *testing.T) {
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

func TestWALTruncatedHeader(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wal")
	w, _, err := OpenWAL(path, 100, "dev", 1)
	if err != nil {
		t.Fatalf("open wal: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := os.Truncate(path, 10); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	if _, _, err := OpenWAL(path, 100, "dev", 1); err == nil {
		t.Fatalf("expected truncated header error")
	}
}

func TestWALPartialWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wal")
	w, _, err := OpenWAL(path, 100, "dev", 1)
	if err != nil {
		t.Fatalf("open wal: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open file: %v", err)
	}
	// Append half of an entry to simulate a crash during write.
	if _, err := f.Seek(walHeaderSize, 0); err != nil {
		t.Fatalf("seek: %v", err)
	}
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], 5)
	if _, err := f.Write(buf[:]); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close file: %v", err)
	}
	w2, ranges, err := OpenWAL(path, 100, "dev", 1)
	if err != nil {
		t.Fatalf("reopen wal: %v", err)
	}
	if len(ranges) != 0 {
		t.Fatalf("expected empty ranges, got %#v", ranges)
	}
	w2.Close()
	// Verify the partial entry was truncated.
	st, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if want := int64(walHeaderSize); st.Size() != want {
		t.Fatalf("expected size %d, got %d", want, st.Size())
	}
}

type shortWriteFile struct{}

func (s *shortWriteFile) ReadAt(p []byte, off int64) (int, error)  { return 0, io.EOF }
func (s *shortWriteFile) Write(p []byte) (int, error)              { return len(p) - 1, nil }
func (s *shortWriteFile) WriteAt(p []byte, off int64) (int, error) { return len(p) - 1, nil }
func (s *shortWriteFile) Seek(int64, int) (int64, error)           { return 0, nil }
func (s *shortWriteFile) Sync() error                              { return nil }
func (s *shortWriteFile) Truncate(int64) error                     { return nil }
func (s *shortWriteFile) Close() error                             { return nil }
func (s *shortWriteFile) Stat() (fs.FileInfo, error)               { return nil, nil }
func (s *shortWriteFile) Name() string                             { return "" }

func TestWALAppendShortWrite(t *testing.T) {
	w := &WAL{f: &shortWriteFile{}}
	if err := w.Append(Range{Start: 0, End: 1}); err == nil {
		t.Fatalf("expected error on short write")
	}
}
