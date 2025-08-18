package ssh

import (
	"context"
	"path/filepath"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"lvmsync_go/transport"
)

func TestAgentSocketUnreachableLogsWarning(t *testing.T) {
	t.Setenv("SSH_AUTH_SOCK", filepath.Join(t.TempDir(), "missing.sock"))
	core, logs := observer.New(zapcore.WarnLevel)
	cfg := transport.Config{Logger: zap.New(core), SSHUser: "u", SSHPassword: "p", SSHUseAgent: true, AllowInsecure: true}
	if _, err := New(context.Background(), cfg); err != nil {
		t.Fatalf("New: %v", err)
	}
	entries := logs.FilterMessage("ssh_agent_unreachable").All()
	if len(entries) != 1 {
		t.Fatalf("expected 1 ssh_agent_unreachable log, got %d", len(entries))
	}
	if entries[0].Level != zapcore.WarnLevel {
		t.Fatalf("expected warn level, got %v", entries[0].Level)
	}
	ctx := entries[0].ContextMap()
	if ctx["transport"] != "ssh" {
		t.Fatalf("expected transport ssh, got %v", ctx["transport"])
	}
	if _, ok := ctx["error"]; !ok {
		t.Fatalf("expected error field in log")
	}
}
