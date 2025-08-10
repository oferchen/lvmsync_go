package main

import (
	"errors"
	"io"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	dumpcmd "lvmsync_go/cmd/dump"
)

type failingSyncCore struct {
	zapcore.Core
	err error
}

func (c *failingSyncCore) Sync() error {
	return c.err
}

func TestCopyPipeAsyncError(t *testing.T) {
	r, w := io.Pipe()
	errCh := dumpcmd.CopyPipeAsync(io.Discard, r)
	expected := errors.New("copy fail")
	w.CloseWithError(expected)
	if err := <-errCh; !errors.Is(err, expected) {
		t.Fatalf("expected %v, got %v", expected, err)
	}
}

func TestSyncLoggerLogsError(t *testing.T) {
	syncErr := errors.New("sync fail")
	core, observed := observer.New(zap.InfoLevel)
	logger := zap.New(&failingSyncCore{Core: core, err: syncErr})
	syncLogger(logger)
	logs := observed.All()
	if len(logs) != 1 {
		t.Fatalf("expected one log entry, got %d", len(logs))
	}
	if logs[0].Message != "Logger sync error" {
		t.Fatalf("unexpected log message %q", logs[0].Message)
	}
	errStr, ok := logs[0].ContextMap()["error"].(string)
	if !ok || errStr != syncErr.Error() {
		t.Fatalf("expected error %q in log, got %v", syncErr.Error(), logs[0].ContextMap()["error"])
	}
}
