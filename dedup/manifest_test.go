package dedup

import (
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestManifestAuditLog(t *testing.T) {
	m := &Manifest{}
	m.Append([32]byte{}, 0, 1)
	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	m.AuditLog(logger)
	if logs.Len() != 1 {
		t.Fatalf("expected one log, got %d", logs.Len())
	}
	entry := logs.All()[0]
	if entry.Message != "session_manifest" {
		t.Fatalf("unexpected message %s", entry.Message)
	}
	if _, ok := entry.ContextMap()["manifest_json"]; !ok {
		t.Fatalf("missing manifest_json field")
	}
}
