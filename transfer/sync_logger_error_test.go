package transfer

import (
	"bytes"
	"errors"
	"sync"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"lvmsync_go/config"
)

// failingSyncCore wraps a zapcore.Core and returns a fixed error from Sync.
type failingSyncCore struct {
	zapcore.Core
	err error
}

func (c *failingSyncCore) Sync() error { return c.err }

func TestDumpChangesLogsSyncError(t *testing.T) {
	syncErr := errors.New("sync fail")
	core, observed := observer.New(zap.InfoLevel)
	logger := zap.New(&failingSyncCore{Core: core, err: syncErr})
	tr := NewTransfer(logger, &sync.WaitGroup{})

	blockSize := int64(1024)
	changed := []int{0}
	src, snapshot := createDumpTestFiles(t, blockSize, changed)
	cfg := &config.Config{BlockSize: int(blockSize), Compress: "none", MaxRetries: 1}
	var buf bytes.Buffer
	if err := tr.DumpChangesSequential(cfg, snapshot, src, &buf); err != nil {
		t.Fatalf("DumpChangesSequential failed: %v", err)
	}
	logs := observed.FilterMessage("Logger sync error").All()
	if len(logs) != 1 {
		t.Fatalf("expected sync error log, got %d", len(logs))
	}
	if errStr, ok := logs[0].ContextMap()["error"].(string); !ok || errStr != syncErr.Error() {
		t.Fatalf("expected error %q in log, got %v", syncErr.Error(), logs[0].ContextMap()["error"])
	}
}
