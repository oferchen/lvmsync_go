package lvmsync

import (
	"os"
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"lvmsync_go/internal/config"
)

func TestEstimateTransferSuccess(t *testing.T) {
	src := t.TempDir() + "/src"
	if err := os.WriteFile(src, []byte("data"), 0o600); err != nil {
		t.Fatalf("write src: %v", err)
	}
	core, obs := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	defer logger.Sync()
	cfg := &config.Config{SpeedLimit: 2}
	if err := estimateTransfer(src, cfg, logger); err != nil {
		t.Fatalf("estimate transfer: %v", err)
	}
	if obs.Len() != 1 {
		t.Fatalf("expected 1 log entry, got %d", obs.Len())
	}
	entry := obs.All()[0]
	if entry.Message != "dry run" {
		t.Fatalf("unexpected log message %q", entry.Message)
	}
	ctx := entry.ContextMap()
	if size := ctx["size_bytes"]; size != int64(4) {
		t.Fatalf("unexpected size %v", size)
	}
	if dur := ctx["estimated_duration_ms"]; dur != int64(2000) {
		t.Fatalf("unexpected duration %v", dur)
	}
	if bw := ctx["estimated_bandwidth_bps"]; bw != int64(16) {
		t.Fatalf("unexpected bandwidth %v", bw)
	}
}

func TestEstimateTransferInvalidPath(t *testing.T) {
	logger := zap.NewNop()
	err := estimateTransfer("/does/not/exist", &config.Config{}, logger)
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "stat source") {
		t.Fatalf("unexpected error %v", err)
	}
}
