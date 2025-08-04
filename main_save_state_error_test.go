package main

import (
	"io"
	"path/filepath"
	"testing"

	"lvmsync_go/config"
	"lvmsync_go/transfer"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestRunClientModeLogsSaveStateError(t *testing.T) {
	cfg = config.DefaultConfig()
	cfg.StdoutMode = true
	cfg.Deduplication = true
	cfg.DedupStrategy = "checksum"
	cfg.DedupStateFile = filepath.Join(t.TempDir(), "missing", "state")
	cfg.BlockSize = 1024
	cfg.MaxRetries = 1

	original := dumpChangesWithDeduplication
	dumpChangesWithDeduplication = func(c *config.Config, snapshot, source string, out io.Writer, dedup transfer.DeduplicationStrategy) error {
		return nil
	}
	defer func() { dumpChangesWithDeduplication = original }()

	core, observed := observer.New(zap.ErrorLevel)
	logger := zap.New(core)
	zap.ReplaceGlobals(logger)
	defer zap.ReplaceGlobals(zap.NewNop())

	if err := runClientMode("/dev/snap", ""); err != nil {
		t.Fatalf("runClientMode returned error: %v", err)
	}

	logs := observed.FilterMessage("Failed to save dedup state").All()
	if len(logs) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(logs))
	}
}
