package transfer

import (
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"

	"lvmsync_go/device"
	"lvmsync_go/internal/config"
)

// TestWALCommitFsync verifies that WAL entries remain after a crash due to fsync.
func TestWALCommitFsync(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wal")
	w, _, err := OpenWAL(path, 128, "dev", 1, nil)
	if err != nil {
		t.Fatalf("open wal: %v", err)
	}
	if err := w.Append(Range{Start: 0, End: 64}); err != nil {
		t.Fatalf("append: %v", err)
	}
	// Simulate crash by closing the underlying file without calling Close.
	if err := w.File().Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	w2, ranges, err := OpenWAL(path, 128, "dev", 1, nil)
	if err != nil {
		t.Fatalf("reopen wal: %v", err)
	}
	if len(ranges) != 1 || ranges[0].Start != 0 || ranges[0].End != 64 {
		t.Fatalf("unexpected ranges %#v", ranges)
	}
	w2.Close()
}

// TestResumeValidation ensures mismatched resume metadata is ignored.
func TestResumeValidation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "resume.json")
	// Write resume state with mismatched metadata.
	state := resumeState{
		Transport:         "ssh",
		Compress:          "none",
		ChecksumAlgorithm: "blake3",
		DedupMode:         "fixed",
		SizeBytes:         1,
		DeviceID:          "src",
		Epoch:             1,
	}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write resume: %v", err)
	}
	cfg := &config.Config{ResumeState: path, DedupMode: "fixed", Transport: "ssh", Compress: "none", ChecksumAlgorithm: "blake3"}
	chk := readResumeState(cfg, zap.NewNop(), device.DeviceIdentity{SizeBytes: 2, FSUUID: "dest", ManifestEpoch: 2}, [32]byte{})
	if rc := chk.chunk("fixed"); rc != (resumeChunk{}) {
		t.Fatalf("expected empty checkpoint, got %#v", rc)
	}
}

// TestResumeDeviceIDMismatch ensures mismatched device IDs invalidate resume state.
func TestResumeDeviceIDMismatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "resume.json")
	state := resumeState{
		Transport:         "ssh",
		Compress:          "none",
		ChecksumAlgorithm: "blake3",
		DedupMode:         "fixed",
		SizeBytes:         2,
		DeviceID:          "src",
		Epoch:             2,
	}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write resume: %v", err)
	}
	cfg := &config.Config{ResumeState: path, DedupMode: "fixed", Transport: "ssh", Compress: "none", ChecksumAlgorithm: "blake3"}
	chk := readResumeState(cfg, zap.NewNop(), device.DeviceIdentity{SizeBytes: 2, FSUUID: "dest", ManifestEpoch: 2}, [32]byte{})
	if rc := chk.chunk("fixed"); rc != (resumeChunk{}) {
		t.Fatalf("expected empty checkpoint, got %#v", rc)
	}
}

// TestWALHeaderCorruption ensures corrupted headers fail to open.
func TestWALHeaderCorruption(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wal")
	w, _, err := OpenWAL(path, 128, "dev", 1, nil)
	if err != nil {
		t.Fatalf("open wal: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatalf("open file: %v", err)
	}
	if _, err := f.WriteAt([]byte{0xff}, 0); err != nil {
		t.Fatalf("corrupt header: %v", err)
	}
	f.Close()
	if _, _, err := OpenWAL(path, 128, "dev", 1, nil); err == nil {
		t.Fatalf("expected header corruption error")
	}
}

// TestWALDeviceIDCorruption ensures tampered device IDs are detected.
func TestWALDeviceIDCorruption(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wal")
	w, _, err := OpenWAL(path, 128, "dev", 1, nil)
	if err != nil {
		t.Fatalf("open wal: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatalf("open file: %v", err)
	}
	if _, err := f.WriteAt([]byte("bad"), 16); err != nil {
		t.Fatalf("corrupt device id: %v", err)
	}
	f.Close()
	if _, _, err := OpenWAL(path, 128, "dev", 1, nil); err == nil {
		t.Fatalf("expected device id corruption error")
	}
}

// TestWALDetectsUnsyncedEntry simulates power loss before fsync.
func TestWALDetectsUnsyncedEntry(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wal")
	w, _, err := OpenWAL(path, 128, "dev", 1, nil)
	if err != nil {
		t.Fatalf("open wal: %v", err)
	}
	if err := w.Append(Range{Start: 0, End: 64}); err != nil {
		t.Fatalf("append: %v", err)
	}
	// Write a partial entry without syncing to simulate a crash before fsync.
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], 64)
	if _, err := w.File().Write(buf[:]); err != nil {
		t.Fatalf("partial write: %v", err)
	}
	if err := w.File().Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	w2, ranges, err := OpenWAL(path, 128, "dev", 1, nil)
	if err != nil {
		t.Fatalf("reopen wal: %v", err)
	}
	if len(ranges) != 1 || ranges[0].Start != 0 || ranges[0].End != 64 {
		t.Fatalf("unexpected ranges %#v", ranges)
	}
	w2.Close()
}
