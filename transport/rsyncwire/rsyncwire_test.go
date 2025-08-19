package rsyncwire

import (
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"lvmsync_go/transport"
)

func TestRequiresAllowInsecure(t *testing.T) {
	if _, err := New(transport.Config{Logger: zap.NewNop()}); err == nil {
		t.Fatalf("expected error when AllowInsecure is false")
	}
}

func TestLogsPlaintextWarning(t *testing.T) {
	core, logs := observer.New(zap.WarnLevel)
	logger := zap.New(core)
	if _, err := New(transport.Config{Logger: logger, AllowInsecure: true}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	entries := logs.TakeAll()
	if len(entries) == 0 {
		t.Fatalf("expected warning log")
	}
	if entries[0].Message != "plaintext_connection" {
		t.Fatalf("unexpected message: %s", entries[0].Message)
	}
	if entries[0].ContextMap()["docs"] != "docs/transports.md" {
		t.Fatalf("missing docs field: %v", entries[0].ContextMap()["docs"])
	}
}
