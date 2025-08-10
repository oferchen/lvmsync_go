package dedup

import (
	"errors"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

type failingSyncCore struct {
	zapcore.Core
	err error
}

func (c *failingSyncCore) Sync() error {
	return c.err
}

func TestAuditLogSuccessSyncError(t *testing.T) {
	m := &Manifest{}
	m.Append([32]byte{}, 0, 1)
	core, observed := observer.New(zap.InfoLevel)
	syncErr := errors.New("sync fail")
	logger := zap.New(&failingSyncCore{Core: core, err: syncErr})
	m.AuditLog(logger)
	entries := observed.All()
	if len(entries) != 2 {
		t.Fatalf("expected 2 log entries, got %d", len(entries))
	}
	if entries[0].Message != "session_manifest" {
		t.Fatalf("unexpected first log message %q", entries[0].Message)
	}
	if entries[1].Message != "Logger sync error" {
		t.Fatalf("unexpected second log message %q", entries[1].Message)
	}
	if errStr, ok := entries[1].ContextMap()["error"].(string); !ok || errStr != syncErr.Error() {
		t.Fatalf("expected sync error %q, got %v", syncErr.Error(), entries[1].ContextMap()["error"])
	}
}

func TestAuditLogMarshalErrorSyncError(t *testing.T) {
	m := &Manifest{}
	original := jsonMarshal
	marshalErr := errors.New("marshal fail")
	jsonMarshal = func(v any) ([]byte, error) { return nil, marshalErr }
	defer func() { jsonMarshal = original }()
	core, observed := observer.New(zap.InfoLevel)
	syncErr := errors.New("sync fail")
	logger := zap.New(&failingSyncCore{Core: core, err: syncErr})
	m.AuditLog(logger)
	entries := observed.All()
	if len(entries) != 2 {
		t.Fatalf("expected 2 log entries, got %d", len(entries))
	}
	if entries[0].Message != "manifest_marshal_error" {
		t.Fatalf("unexpected first log message %q", entries[0].Message)
	}
	if errStr, ok := entries[0].ContextMap()["error"].(string); !ok || errStr != marshalErr.Error() {
		t.Fatalf("expected marshal error %q, got %v", marshalErr.Error(), entries[0].ContextMap()["error"])
	}
	if entries[1].Message != "Logger sync error" {
		t.Fatalf("unexpected second log message %q", entries[1].Message)
	}
	if errStr, ok := entries[1].ContextMap()["error"].(string); !ok || errStr != syncErr.Error() {
		t.Fatalf("expected sync error %q, got %v", syncErr.Error(), entries[1].ContextMap()["error"])
	}
}
