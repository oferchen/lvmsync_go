package transfer

import (
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"lvmsync_go/config"
)

func TestFinalizeProgress(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	Logger = zap.New(core)
	cfg := &config.Config{Progress: true}
	finalizeProgress(cfg)
	if logs.Len() != 0 {
		t.Fatalf("expected no logs, got %d", logs.Len())
	}
}

func TestReportProgress(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	Logger = zap.New(core)
	cfg := &config.Config{Progress: true}
	reportProgress(cfg, 50, 100, 1, time.Now())
	if logs.Len() == 0 {
		t.Fatal("expected progress log entry")
	}
}

func TestLogSummaries(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	Logger = zap.New(core)
	start := time.Now().Add(-time.Second)
	logSequentialSummary(1024, 1, start)
	logParallelSummary(2048, start)
	if logs.Len() != 2 {
		t.Fatalf("expected 2 log entries, got %d", logs.Len())
	}
}
