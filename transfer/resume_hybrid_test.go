package transfer

import (
	"path/filepath"
	"testing"

	"github.com/zeebo/blake3"
	"go.uber.org/zap"

	"lvmsync_go/config"
)

func TestResumeHybridOffset(t *testing.T) {
	logger := zap.NewNop()
	dir := t.TempDir()
	state := filepath.Join(dir, "resume")

	cfg := &config.Config{ResumeState: state, Compress: "none", ChecksumAlgorithm: "blake3", Transport: "ssh", DedupMode: "hybrid"}
	digest := blake3.Sum256([]byte("chunk"))
	writeResumeState(cfg, logger, state, 80, 40, digest)

	chk := readResumeState(cfg, logger)
	ranges := []Range{{Start: 0, End: 99}, {Start: 100, End: 199}}
	idx := findResumeIndex(cfg, nil, ranges, chk, logger)
	if idx != 1 {
		t.Fatalf("expected index 1, got %d", idx)
	}
	if ranges[1].Start != 120 {
		t.Fatalf("expected range start 120, got %d", ranges[1].Start)
	}
}
