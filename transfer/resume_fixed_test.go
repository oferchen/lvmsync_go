package transfer

import (
	"bytes"
	"context"
	"sync"
	"testing"

	"github.com/zeebo/blake3"
	"go.uber.org/zap"

	"lvmsync_go/internal/config"
)

func TestResumeFixedIdempotent(t *testing.T) {
	tr := NewTransfer(zap.NewNop(), &sync.WaitGroup{}, nil)
	blockSize := int64(1024)
	src, snapshot, resume := createTestFiles(t, blockSize, 4, "blake3")

	cfg := &config.Config{BlockSize: int(blockSize), Compress: "none", Parallel: 1, ResumeState: resume, MaxRetries: 1, ChecksumAlgorithm: "blake3", Transport: "ssh", DedupMode: "fixed"}

	digest := blake3.Sum256(bytes.Repeat([]byte{2}, int(blockSize)))
	var first, second bytes.Buffer
	for i := 0; i < 2; i++ {
		tr.Tracker = &resumeTracker{}
		writeResumeState(cfg, zap.NewNop(), resume, resumeChunks{Fixed: resumeChunk{Chunk: digest, Offset: uint64(blockSize), Length: uint32(blockSize)}}, 0, "", 0, [32]byte{})
		buf := &first
		if i == 1 {
			buf = &second
		}
		if err := tr.DumpChangesParallel(context.Background(), cfg, snapshot, src, buf); err != nil {
			t.Fatalf("DumpChangesParallel failed: %v", err)
		}
		finalizeResumeState(cfg, tr.Tracker, zap.NewNop())
	}
	if !bytes.Equal(first.Bytes(), second.Bytes()) {
		t.Fatalf("resume output mismatch")
	}
}
