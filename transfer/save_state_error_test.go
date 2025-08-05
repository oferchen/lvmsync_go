package transfer

import (
	"bytes"
	"path/filepath"
	"testing"

	"lvmsync_go/config"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestDumpChangesLogsSaveStateError(t *testing.T) {
	core, observed := observer.New(zap.ErrorLevel)
	logger := zap.New(core)
	SetLogger(logger)
	defer SetLogger(nil)

	blockSize := int64(1024)
	changed := []int{0}
	src, snapshot := createDumpTestFiles(t, blockSize, changed)

	cfg := &config.Config{BlockSize: int(blockSize), Compress: "none", DedupStrategy: "checksum", DedupStateFile: filepath.Join(t.TempDir(), "missing", "state"), MaxRetries: 1}
	var buf bytes.Buffer
	if err := DumpChanges(cfg, snapshot, src, &buf); err != nil {
		t.Fatalf("DumpChanges failed: %v", err)
	}
	logs := observed.FilterMessage("Failed to save dedup state").All()
	if len(logs) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(logs))
	}
}
