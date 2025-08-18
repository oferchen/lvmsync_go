package transfer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"

	"lvmsync_go/internal/config"
)

// TestWALCommitFsync verifies that WAL entries remain after a crash due to fsync.
func TestWALCommitFsync(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wal")
	w, _, err := OpenWAL(path, 128, "dev", 1)
	if err != nil {
		t.Fatalf("open wal: %v", err)
	}
	if err := w.Append(Range{Start: 0, End: 64}); err != nil {
		t.Fatalf("append: %v", err)
	}
	// Simulate crash by closing the underlying file without calling Close.
	if err := w.f.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	w2, ranges, err := OpenWAL(path, 128, "dev", 1)
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
	chk := readResumeState(cfg, zap.NewNop(), 2, "dest", 2)
	if rc := chk.chunk("fixed"); rc != (resumeChunk{}) {
		t.Fatalf("expected empty checkpoint, got %#v", rc)
	}
}
