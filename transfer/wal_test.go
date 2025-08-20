package transfer

import (
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestWALMismatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wal")
	w, _, err := OpenWAL(path, 100, "dev", 1, nil)
	if err != nil {
		t.Fatalf("open wal: %v", err)
	}
	w.Close()
	if _, _, err := OpenWAL(path, 101, "dev", 1, nil); err == nil {
		t.Fatalf("expected size mismatch error")
	}
	if _, _, err := OpenWAL(path, 100, "dev2", 1, nil); err == nil {
		t.Fatalf("expected device mismatch error")
	}
	if _, _, err := OpenWAL(path, 100, "dev", 2, nil); err == nil {
		t.Fatalf("expected epoch mismatch error")
	}
}

func TestWALRecovery(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wal")
	w, _, err := OpenWAL(path, 100, "dev", 1, nil)
	if err != nil {
		t.Fatalf("open wal: %v", err)
	}
	if err := w.Append(Range{Start: 0, End: 10}); err != nil {
		t.Fatalf("append: %v", err)
	}
	w.Close()
	w, ranges, err := OpenWAL(path, 100, "dev", 1, nil)
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
	w, _, err := OpenWAL(path, 100, "dev", 1, nil)
	if err != nil {
		t.Fatalf("open wal: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := os.Truncate(path, 10); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	if _, _, err := OpenWAL(path, 100, "dev", 1, nil); err == nil {
		t.Fatalf("expected truncated header error")
	}
}

func TestWALPartialWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wal")
	w, _, err := OpenWAL(path, 100, "dev", 1, nil)
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
	w2, ranges, err := OpenWAL(path, 100, "dev", 1, nil)
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

func TestWALSyncDirAppend(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wal")
	w, _, err := OpenWAL(path, 100, "dev", 1, nil)
	if err != nil {
		t.Fatalf("open wal: %v", err)
	}
	stubErr := errors.New("syncdir fail")
	var calls int
	w.deps = &WALDeps{syncDir: func(string) error {
		calls++
		return stubErr
	}}
	if err := w.Append(Range{Start: 0, End: 1}); !errors.Is(err, stubErr) {
		t.Fatalf("expected %v got %v", stubErr, err)
	}
	if calls != 1 {
		t.Fatalf("expected syncDir called once, got %d", calls)
	}
	w.Close()
}
