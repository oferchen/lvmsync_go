package transfer

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/zeebo/blake3"
	"go.uber.org/zap"

	"lvmsync_go/internal/config"
)

func TestSaveAndReadResumeState(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "resume.json")
	cfg := &config.Config{
		Transport:         "ssh",
		Compress:          "none",
		ChecksumAlgorithm: "blake3",
		ResumeState:       path,
		DedupMode:         "fixed",
		CheckpointBytes:   4,
	}
	rt := &resumeTracker{sizeBytes: 100, deviceID: "id", epoch: 1}
	digest := blake3.Sum256([]byte("data"))
	cfg.FirstBlockDigest = hex.EncodeToString(digest[:])
	saveResumeState(cfg, rt, 0, digest, 4, zap.NewNop())
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected resume state file: %v", err)
	}
	cp := readResumeState(cfg, zap.NewNop(), 100, "id", 1, digest)
	rc := cp.chunk("fixed")
	if rc.Offset != 0 || rc.Length != 4 || hex.EncodeToString(rc.Chunk[:]) != hex.EncodeToString(digest[:]) {
		t.Fatalf("unexpected checkpoint: %+v", rc)
	}
	finalizeResumeState(cfg, rt, zap.NewNop())
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("resume state not removed: %v", err)
	}
}

func TestReadResumeStateDigestMismatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "resume.json")
	good := blake3.Sum256([]byte("data"))
	cfg := &config.Config{
		Transport:         "ssh",
		Compress:          "none",
		ChecksumAlgorithm: "blake3",
		ResumeState:       path,
		DedupMode:         "fixed",
		CheckpointBytes:   4,
		FirstBlockDigest:  hex.EncodeToString(good[:]),
	}
	rt := &resumeTracker{}
	saveResumeState(cfg, rt, 0, good, 4, zap.NewNop())
	bad := blake3.Sum256([]byte("other"))
	cp := readResumeState(cfg, zap.NewNop(), 0, cfg.DeviceUUID, 0, bad)
	if cp != (resumeCheckpoint{}) {
		t.Fatalf("expected empty checkpoint on digest mismatch")
	}
}

func TestReadResumeStateSizeMismatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "resume.json")
	dig := blake3.Sum256([]byte("data"))
	cfg := &config.Config{Transport: "ssh", Compress: "none", ChecksumAlgorithm: "blake3", ResumeState: path, DedupMode: "fixed", CheckpointBytes: 4, FirstBlockDigest: hex.EncodeToString(dig[:])}
	rt := &resumeTracker{sizeBytes: 100, deviceID: "id", epoch: 1}
	saveResumeState(cfg, rt, 0, dig, 4, zap.NewNop())
	cp := readResumeState(cfg, zap.NewNop(), 200, "id", 1, dig)
	if cp != (resumeCheckpoint{}) {
		t.Fatalf("expected empty checkpoint on size mismatch")
	}
}

func TestReadResumeStateEpochMismatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "resume.json")
	dig := blake3.Sum256([]byte("data"))
	cfg := &config.Config{Transport: "ssh", Compress: "none", ChecksumAlgorithm: "blake3", ResumeState: path, DedupMode: "fixed", CheckpointBytes: 4, FirstBlockDigest: hex.EncodeToString(dig[:])}
	rt := &resumeTracker{sizeBytes: 100, deviceID: "id", epoch: 1}
	saveResumeState(cfg, rt, 0, dig, 4, zap.NewNop())
	cp := readResumeState(cfg, zap.NewNop(), 100, "id", 2, dig)
	if cp != (resumeCheckpoint{}) {
		t.Fatalf("expected empty checkpoint on epoch mismatch")
	}
}
