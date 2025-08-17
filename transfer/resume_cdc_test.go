package transfer

import (
	"bytes"
	"context"
	"path/filepath"
	"reflect"
	"sort"
	"sync"
	"testing"

	"github.com/zeebo/blake3"
	"go.uber.org/zap"

	"lvmsync_go/internal/config"
)

func TestResumeCDCOffset(t *testing.T) {
	logger := zap.NewNop()
	dir := t.TempDir()
	state := filepath.Join(dir, "resume")

	cfg := &config.Config{ResumeState: state, Compress: "none", ChecksumAlgorithm: "blake3", Transport: "ssh", DedupMode: "cdc"}
	digest := blake3.Sum256([]byte("chunk"))
	writeResumeState(cfg, logger, state, resumeChunks{CDC: resumeChunk{Chunk: digest, Offset: 80, Length: 40}})

	chk := readResumeState(cfg, logger)
	ranges := []Range{{Start: 0, End: 99}, {Start: 100, End: 199}}
	idx := findResumeIndex(context.Background(), cfg, nil, ranges, chk, logger)
	if idx != 1 {
		t.Fatalf("expected index 1, got %d", idx)
	}
	if ranges[1].Start != 120 {
		t.Fatalf("expected range start 120, got %d", ranges[1].Start)
	}
}

func TestResumeCDCSequential(t *testing.T) {
	tr := NewTransfer(zap.NewNop(), &sync.WaitGroup{}, nil)
	tr.Tracker = &resumeTracker{}
	blockSize := int64(1024)
	src, snapshot, resume := createTestFiles(t, blockSize, 4, "blake3")

	cfg := &config.Config{BlockSize: int(blockSize), Compress: "none", Parallel: 1, ResumeState: resume, MaxRetries: 1, ChecksumAlgorithm: "blake3", Transport: "ssh", DedupMode: "cdc"}

	digest := blake3.Sum256(bytes.Repeat([]byte{2}, int(blockSize)))
	writeResumeState(cfg, zap.NewNop(), resume, resumeChunks{CDC: resumeChunk{Chunk: digest, Offset: uint64(blockSize), Length: uint32(blockSize)}})

	var buf bytes.Buffer
	if err := tr.DumpChangesParallel(context.Background(), cfg, snapshot, src, &buf); err != nil {
		t.Fatalf("DumpChangesParallel failed: %v", err)
	}
	finalizeResumeState(cfg, tr.Tracker, zap.NewNop())

	offsets := parseOffsets(t, buf.Bytes(), blockSize)
	sort.Slice(offsets, func(i, j int) bool { return offsets[i] < offsets[j] })
	expected := []int64{2 * blockSize, 3 * blockSize}
	if !reflect.DeepEqual(offsets, expected) {
		t.Fatalf("unexpected offsets %v, want %v", offsets, expected)
	}
}

func TestResumeCDCIdempotent(t *testing.T) {
	tr := NewTransfer(zap.NewNop(), &sync.WaitGroup{}, nil)
	blockSize := int64(1024)
	src, snapshot, resume := createTestFiles(t, blockSize, 4, "blake3")
	cfg := &config.Config{BlockSize: int(blockSize), Compress: "none", Parallel: 1, ResumeState: resume, MaxRetries: 1, ChecksumAlgorithm: "blake3", Transport: "ssh", DedupMode: "cdc"}
	digest := blake3.Sum256(bytes.Repeat([]byte{2}, int(blockSize)))
	var first, second bytes.Buffer
	for i := 0; i < 2; i++ {
		tr.Tracker = &resumeTracker{}
		writeResumeState(cfg, zap.NewNop(), resume, resumeChunks{CDC: resumeChunk{Chunk: digest, Offset: uint64(blockSize), Length: uint32(blockSize)}})
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
