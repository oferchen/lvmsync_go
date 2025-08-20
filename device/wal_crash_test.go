package device

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"
)

// TestWALCrashRecovery simulates crash scenarios and ensures WAL recovery.
func TestWALCrashRecovery(t *testing.T) {
	id := DeviceIdentity{SizeBytes: 128, KernelUUID: "k", GPTUUID: "g", MBRSignature: "", FSUUID: "f", Major: 1, Minor: 2, ManifestEpoch: 1}

	t.Run("partial_header", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "wal")
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		var buf [8]byte
		binary.LittleEndian.PutUint64(buf[:], walVersion)
		if _, err := f.Write(buf[:]); err != nil {
			t.Fatalf("write: %v", err)
		}
		if err := f.Close(); err != nil {
			t.Fatalf("close: %v", err)
		}
		w, err := OpenWAL(path, id, zap.NewNop(), nil)
		if err != nil {
			t.Fatalf("reopen wal: %v", err)
		}
		if len(w.Ranges()) != 0 {
			t.Fatalf("expected no ranges, got %#v", w.Ranges())
		}
		w.Close()
	})

	t.Run("truncate_mid_entry", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "wal")
		w, err := OpenWAL(path, id, zap.NewNop(), nil)
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
		w2, err := OpenWAL(path, id, zap.NewNop(), nil)
		if err != nil {
			t.Fatalf("reopen wal: %v", err)
		}
		if len(w2.Ranges()) != 0 {
			t.Fatalf("expected no ranges, got %#v", w2.Ranges())
		}
		w2.Close()
	})

	t.Run("omit_fsync", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "wal")
		w, err := OpenWAL(path, id, zap.NewNop(), nil)
		if err != nil {
			t.Fatalf("open wal: %v", err)
		}
		if err := w.Append(Range{Start: 0, End: 64}); err != nil {
			t.Fatalf("append: %v", err)
		}
		var entry [16]byte
		binary.LittleEndian.PutUint64(entry[0:8], 64)
		binary.LittleEndian.PutUint64(entry[8:16], 128)
		if _, err := w.f.Write(entry[:]); err != nil {
			t.Fatalf("write entry: %v", err)
		}
		if err := w.f.Close(); err != nil {
			t.Fatalf("close: %v", err)
		}
		if err := os.Truncate(path, walHeaderSize+16); err != nil {
			t.Fatalf("truncate: %v", err)
		}
		w2, err := OpenWAL(path, id, zap.NewNop(), nil)
		if err != nil {
			t.Fatalf("reopen wal: %v", err)
		}
		rs := w2.Ranges()
		if len(rs) != 1 || rs[0].Start != 0 || rs[0].End != 64 {
			t.Fatalf("unexpected ranges %#v", rs)
		}
		w2.Close()
	})

	t.Run("truncate_within_entry", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "wal")
		w, err := OpenWAL(path, id, zap.NewNop(), nil)
		if err != nil {
			t.Fatalf("open wal: %v", err)
		}
		if err := w.Append(Range{Start: 0, End: 64}); err != nil {
			t.Fatalf("append: %v", err)
		}
		var entry [16]byte
		binary.LittleEndian.PutUint64(entry[0:8], 64)
		binary.LittleEndian.PutUint64(entry[8:16], 128)
		if _, err := w.f.Write(entry[:]); err != nil {
			t.Fatalf("write entry: %v", err)
		}
		if err := w.f.Close(); err != nil {
			t.Fatalf("close: %v", err)
		}
		if err := os.Truncate(path, walHeaderSize+16+8); err != nil {
			t.Fatalf("truncate: %v", err)
		}
		w2, err := OpenWAL(path, id, zap.NewNop(), nil)
		if err != nil {
			t.Fatalf("reopen wal: %v", err)
		}
		rs := w2.Ranges()
		if len(rs) != 1 || rs[0].Start != 0 || rs[0].End != 64 {
			t.Fatalf("unexpected ranges %#v", rs)
		}
		if err := w2.Append(Range{Start: 64, End: 128}); err != nil {
			t.Fatalf("append: %v", err)
		}
		rs = w2.Ranges()
		if len(rs) != 2 || rs[0].Start != 0 || rs[0].End != 64 || rs[1].Start != 64 || rs[1].End != 128 {
			t.Fatalf("unexpected ranges after resume %#v", rs)
		}
		w2.Close()
	})
}
