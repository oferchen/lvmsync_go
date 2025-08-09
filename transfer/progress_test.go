package transfer

import (
	"io"
	"os"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"lvmsync_go/config"
)

func captureStderr(f func()) string {
	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	f()
	w.Close()
	os.Stderr = old
	out, _ := io.ReadAll(r)
	return string(out)
}

func TestFinalizeProgress(t *testing.T) {
	cfg := &config.Config{Progress: true}
	out := captureStderr(func() { finalizeProgress(cfg) })
	if out != "\n" {
		t.Fatalf("expected newline, got %q", out)
	}
}

func TestReportProgress(t *testing.T) {
	Logger = zap.NewNop()
	cfg := &config.Config{Progress: true}
	out := captureStderr(func() {
		reportProgress(cfg, 50, 100, 1, time.Now())
	})
	if out == "" {
		t.Fatal("expected progress output")
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
