package transfer

import (
	"bytes"
	"context"
	"encoding/hex"
	"path/filepath"
	"sync"
	"testing"

	"github.com/zeebo/blake3"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/oferchen/lvmsync_go/internal/config"
)

func TestDumpChangesSequentialLogFields(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	tr := NewTransfer(zap.New(core), &sync.WaitGroup{}, nil)

	blockSize := int64(1024)
	changed := []int{0, 2}
	src, snapshot := createDumpTestFiles(t, blockSize, changed)

	cfg := &config.Config{BlockSize: int(blockSize), Compress: "none", MaxRetries: 1}
	var buf bytes.Buffer
	if err := tr.DumpChangesSequential(context.Background(), cfg, snapshot, src, &buf); err != nil {
		t.Fatalf("DumpChangesSequential failed: %v", err)
	}

	using := logs.FilterMessage("Using block size").All()
	if len(using) != 1 {
		t.Fatalf("expected one 'Using block size' log, got %d", len(using))
	}
	if v, ok := using[0].ContextMap()["block_size_bytes"].(int64); !ok || v != blockSize {
		t.Fatalf("expected block_size_bytes=%d, got %v", blockSize, using[0].ContextMap()["block_size_bytes"])
	}

	changedLogs := logs.FilterMessage("Changed blocks determined").All()
	if len(changedLogs) != 1 {
		t.Fatalf("expected one 'Changed blocks determined' log, got %d", len(changedLogs))
	}
	if v, ok := changedLogs[0].ContextMap()["block_count"].(int64); !ok || int(v) != len(changed) {
		t.Fatalf("expected block_count=%d, got %v", len(changed), changedLogs[0].ContextMap()["block_count"])
	}
}

func TestReadResumeDigestLogField(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core)

	tmp := t.TempDir()
	stateFile := filepath.Join(tmp, "resume")
	digest := blake3.Sum256([]byte("data"))
	cfg := &config.Config{ResumeState: stateFile, Compress: "none", ChecksumAlgorithm: "blake3", DedupMode: "fixed"}
	chk := resumeChunk{Chunk: digest, Offset: 1024, Length: 2048}
	writeResumeState(cfg, logger, stateFile, resumeChunks{Fixed: chk}, 0, "", 0)

	val := readResumeState(cfg, logger, 0, cfg.DeviceUUID, 0, [32]byte{}).Fixed
	if val.Chunk != digest {
		t.Fatalf("expected digest match")
	}

	entries := logs.FilterMessage("Resuming from chunk").All()
	if len(entries) != 1 {
		t.Fatalf("expected one resume log, got %d", len(entries))
	}
	if v, ok := entries[0].ContextMap()["resume_chunk"].(string); !ok || v != hex.EncodeToString(digest[:]) {
		t.Fatalf("unexpected resume_chunk %v", entries[0].ContextMap()["resume_chunk"])
	}
	if v, ok := entries[0].ContextMap()["resume_offset_bytes"].(uint64); !ok || v != chk.Offset {
		t.Fatalf("unexpected resume_offset_bytes %v", entries[0].ContextMap()["resume_offset_bytes"])
	}
	if v, ok := entries[0].ContextMap()["resume_length_bytes"].(uint32); !ok || v != chk.Length {
		t.Fatalf("unexpected resume_length_bytes %v", entries[0].ContextMap()["resume_length_bytes"])
	}
}
