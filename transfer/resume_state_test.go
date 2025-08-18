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
	rt := &resumeTracker{}
	digest := blake3.Sum256([]byte("data"))
	saveResumeState(cfg, rt, 0, digest, 4, zap.NewNop())
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected resume state file: %v", err)
	}
	cp := readResumeState(cfg, zap.NewNop(), 0, cfg.DeviceUUID, 0)
	rc := cp.chunk("fixed")
	if rc.Offset != 0 || rc.Length != 4 || hex.EncodeToString(rc.Chunk[:]) != hex.EncodeToString(digest[:]) {
		t.Fatalf("unexpected checkpoint: %+v", rc)
	}
	finalizeResumeState(cfg, rt, zap.NewNop())
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("resume state not removed: %v", err)
	}
}
