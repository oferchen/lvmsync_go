package transfer

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"lvmsync_go/config"
)

func TestDumpChangesSequentialLogFields(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	SetLogger(zap.New(core))

	blockSize := int64(1024)
	changed := []int{0, 2}
	src, snapshot := createDumpTestFiles(t, blockSize, changed)

	cfg := &config.Config{BlockSize: int(blockSize), Compress: "none", MaxRetries: 1}
	var buf bytes.Buffer
	if err := DumpChangesSequential(cfg, snapshot, src, &buf); err != nil {
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

func TestReadResumeStartLogField(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	Logger = zap.New(core)

	tmp := t.TempDir()
	stateFile := filepath.Join(tmp, "resume")
	if err := os.WriteFile(stateFile, []byte("5"), 0o600); err != nil {
		t.Fatalf("write resume state: %v", err)
	}

	cfg := &config.Config{ResumeState: stateFile}
	val := readResumeStart(cfg)
	if val != 5 {
		t.Fatalf("expected resume value 5, got %d", val)
	}

	entries := logs.FilterMessage("Resuming from block").All()
	if len(entries) != 1 {
		t.Fatalf("expected one resume log, got %d", len(entries))
	}
	if v, ok := entries[0].ContextMap()["resume_start_block"].(int64); !ok || v != 5 {
		t.Fatalf("expected resume_start_block=5, got %v", entries[0].ContextMap()["resume_start_block"])
	}
}
