package transfer

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// TestWALCrashRecovery simulates power loss scenarios and verifies WAL recovery.
func TestWALCrashRecovery(t *testing.T) {
	t.Run("truncate_mid_entry", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "wal")
		w, _, err := OpenWAL(path, 128, "dev", 1, nil)
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
		if _, err := f.Seek(walHeaderSize, 0); err != nil {
			t.Fatalf("seek: %v", err)
		}
		var buf [8]byte
		binary.LittleEndian.PutUint64(buf[:], 64)
		if _, err := f.Write(buf[:]); err != nil {
			t.Fatalf("partial write: %v", err)
		}
		if err := f.Close(); err != nil {
			t.Fatalf("close file: %v", err)
		}
		w2, ranges, err := OpenWAL(path, 128, "dev", 1, nil)
		if err != nil {
			t.Fatalf("reopen wal: %v", err)
		}
		if len(ranges) != 0 {
			t.Fatalf("expected no ranges, got %#v", ranges)
		}
		w2.Close()
	})

	t.Run("omit_fsync", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "wal")
		w, _, err := OpenWAL(path, 128, "dev", 1, nil)
		if err != nil {
			t.Fatalf("open wal: %v", err)
		}
		if err := w.Append(Range{Start: 0, End: 64}); err != nil {
			t.Fatalf("append: %v", err)
		}
		var entry [16]byte
		binary.LittleEndian.PutUint64(entry[0:8], 64)
		binary.LittleEndian.PutUint64(entry[8:16], 128)
		if _, err := w.File().Write(entry[:]); err != nil {
			t.Fatalf("write entry: %v", err)
		}
		if err := w.File().Close(); err != nil {
			t.Fatalf("close: %v", err)
		}
		if err := os.Truncate(path, walHeaderSize+16); err != nil {
			t.Fatalf("truncate: %v", err)
		}
		w2, ranges, err := OpenWAL(path, 128, "dev", 1, nil)
		if err != nil {
			t.Fatalf("reopen wal: %v", err)
		}
		if len(ranges) != 1 || ranges[0].Start != 0 || ranges[0].End != 64 {
			t.Fatalf("unexpected ranges %#v", ranges)
		}
		w2.Close()
	})
}
