package transfer

import (
	"bytes"
	"context"
	"path/filepath"
	"sync"
	"testing"

	"lvmsync_go/internal/config"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestDumpChangesLogsSaveStateError(t *testing.T) {
	core, observed := observer.New(zap.ErrorLevel)
	logger := zap.New(core)
	tr := NewTransfer(logger, &sync.WaitGroup{})

	blockSize := int64(1024)
	changed := []int{0}
	src, snapshot := createDumpTestFiles(t, blockSize, changed)

	cfg := &config.Config{BlockSize: int(blockSize), Compress: "none", DedupStrategy: "checksum", DedupStateFile: filepath.Join(t.TempDir(), "missing", "state"), MaxRetries: 1}
	var buf bytes.Buffer
	if dumpErr := tr.DumpChanges(context.Background(), cfg, snapshot, src, &buf); dumpErr != nil {
		t.Fatalf("DumpChanges failed: %v", dumpErr)
	}
	logs := observed.FilterMessage("Failed to save dedup state").All()
	if len(logs) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(logs))
	}
}
