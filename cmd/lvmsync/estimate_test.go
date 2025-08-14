package lvmsync

import (
	"os"
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"lvmsync_go/config"
)

func TestEstimateTransferSuccess(t *testing.T) {
	src := t.TempDir() + "/src"
	if err := os.WriteFile(src, []byte("data"), 0o600); err != nil {
		t.Fatalf("write src: %v", err)
	}
	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	defer logger.Sync()
	cfg := &config.Config{}
	if err := estimateTransfer(src, cfg, logger); err != nil {
		t.Fatalf("estimate transfer: %v", err)
	}
	if logs.Len() != 1 {
		t.Fatalf("expected 1 log entry, got %d", logs.Len())
	}
	entry := logs.All()[0]
	if entry.Message != "dry run" {
		t.Fatalf("unexpected log message %q", entry.Message)
	}
	if size := entry.ContextMap()["size_bytes"]; size != int64(4) {
		t.Fatalf("unexpected size %v", size)
	}
}

func TestEstimateTransferInvalidPath(t *testing.T) {
	core, _ := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	defer logger.Sync()
	err := estimateTransfer("/does/not/exist", &config.Config{}, logger)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "stat source") {
		t.Fatalf("unexpected error %v", err)
	}
}
