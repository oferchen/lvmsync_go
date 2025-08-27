//go:build rsync

package rsyncwire

import (
	"sync"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/oferchen/lvmsync_go/transport"
)

func TestRequiresAllowInsecure(t *testing.T) {
	if _, err := New(transport.Config{Logger: zap.NewNop()}); err == nil {
		t.Fatalf("expected error when AllowInsecure is false")
	}
}

func TestRequiresLogger(t *testing.T) {
	if _, err := New(transport.Config{AllowInsecure: true}); err == nil {
		t.Fatalf("expected error when logger is nil")
	}
}

func TestLogsPlaintextWarning(t *testing.T) {
	plaintextWarnOnce = sync.Once{}
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

func TestPlaintextWarningOnce(t *testing.T) {
	plaintextWarnOnce = sync.Once{}
	core, logs := observer.New(zap.WarnLevel)
	logger := zap.New(core)
	cfg := transport.Config{Logger: logger, AllowInsecure: true}
	if _, err := New(cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := New(cfg); err != nil {
		t.Fatalf("unexpected error on second call: %v", err)
	}
	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("expected 1 warning log, got %d", len(entries))
	}
	if entries[0].Message != "plaintext_connection" {
		t.Fatalf("unexpected message: %s", entries[0].Message)
	}
}
