package transfer

import (
	"encoding/json"
	"io"
	"os"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"lvmsync_go/internal/config"
)

func TestFinalizeProgress(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	cfg := &config.Config{Progress: true}
	finalizeProgress(cfg, logger)
	if logs.FilterMessage("progress complete").Len() != 1 {
		t.Fatalf("expected progress completion log, got %d", logs.FilterMessage("progress complete").Len())
	}
}

func TestReportProgress(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	cfg := &config.Config{Progress: true}
	reportProgress(cfg, 50, 100, 1, time.Now(), logger)
	if logs.FilterMessage("transfer progress").Len() == 0 {
		t.Fatal("expected progress log")
	}
}

func TestReportProgressVerbose(t *testing.T) {
	core, logs := observer.New(zap.DebugLevel)
	logger := zap.New(core)
	cfg := &config.Config{Progress: true, Verbose: 1}
	start := time.Now().Add(-time.Second)
	reportProgress(cfg, 1024, 2048, 100, start, logger)
	if logs.FilterMessage("transfer progress").Len() != 1 {
		t.Fatalf("expected transfer progress log, got %d", logs.FilterMessage("transfer progress").Len())
	}
	if logs.FilterMessage("parallel dump progress").Len() != 1 {
		t.Fatalf("expected parallel dump progress log, got %d", logs.FilterMessage("parallel dump progress").Len())
	}
}

func TestReportProgressJSON(t *testing.T) {
	r, w, _ := os.Pipe()
	old := os.Stdout
	os.Stdout = w
	core, _ := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	cfg := &config.Config{Progress: true, Output: "json"}
	reportProgress(cfg, 50, 100, 1, time.Now(), logger)
	w.Close()
	os.Stdout = old
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var ev progressEvent
	if err := json.Unmarshal(out, &ev); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if ev.Event != "progress" || ev.Progress != 50 {
		t.Fatalf("unexpected event %+v", ev)
	}
}

func TestFinalizeProgressJSON(t *testing.T) {
	r, w, _ := os.Pipe()
	old := os.Stdout
	os.Stdout = w
	core, _ := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	cfg := &config.Config{Progress: true, Output: "json"}
	finalizeProgress(cfg, logger)
	w.Close()
	os.Stdout = old
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var ev progressEvent
	if err := json.Unmarshal(out, &ev); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if ev.Event != "complete" || ev.Progress != 100 {
		t.Fatalf("unexpected event %+v", ev)
	}
}

func TestLogSummaries(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	start := time.Now().Add(-time.Second)
	logSequentialSummary(logger, 1024, 1, start)
	logParallelSummary(logger, 2048, start)
	if logs.Len() != 2 {
		t.Fatalf("expected 2 log entries, got %d", logs.Len())
	}
}
