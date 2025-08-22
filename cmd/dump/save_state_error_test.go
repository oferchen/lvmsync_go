package dump

import (
	"context"
	"io"
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/oferchen/lvmsync_go/device"
	"github.com/oferchen/lvmsync_go/internal/config"
	"github.com/oferchen/lvmsync_go/internal/privilege"
	"github.com/oferchen/lvmsync_go/transfer"
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
	dumpChangesWithDeduplication = func(_ context.Context, _ *transfer.Transfer, c *config.Config, snapshot, source string, out io.Writer, dedup transfer.DeduplicationStrategy) error {
		return nil
	}
	defer func() { dumpChangesWithDeduplication = original }()

	core, observed := observer.New(zap.ErrorLevel)
	logger := zap.New(core)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	origDetect := detectDevice
	detectDevice = func(context.Context, string, bool, string, string, string, string, time.Duration, time.Duration, privilege.Escalator, *zap.Logger, *device.Runner) (device.Device, error) {
		return &fakeDevice{path: "/dev/snap"}, nil
	}
	defer func() { detectDevice = origDetect }()
	if _, err := Run(ctx, cfg, "/dev/snap", "", logger); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	logs := observed.FilterMessage("Failed to save dedup state").All()
	if len(logs) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(logs))
	}
}
