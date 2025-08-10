package dump

import (
	"io"
	"path/filepath"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"lvmsync_go/config"
	"lvmsync_go/transfer"
)

func TestRunLogsSaveStateError(t *testing.T) {
	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig returned error: %v", err)
	}
	cfg.StdoutMode = true
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

	if err := Run(cfg, "/dev/snap", "", logger); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	logs := observed.FilterMessage("Failed to save dedup state").All()
	if len(logs) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(logs))
	}
}
