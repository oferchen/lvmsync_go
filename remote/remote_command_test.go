package remote

import (
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestValidateRemoteCommand(t *testing.T) {
	_, client := newSSHServerClient(t, func(cmd string) int {
		switch cmd {
		case "echo --version":
			return 0
		case "nonexistent --version":
			return 127
		default:
			return 0
		}
	})

	if err := ValidateRemoteCommand(client, "echo"); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if err := ValidateRemoteCommand(client, "nonexistent"); err == nil {
		t.Fatalf("expected error for nonexistent command")
	}
}

func TestRunRemoteScript(t *testing.T) {
	server, client := newSSHServerClient(t, func(_ string) int { return 0 }) // cmd is unused

	core, observed := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	SetLogger(logger)
	defer SetLogger(nil)

	script := "echo hi"
	if err := RunRemoteScript(client, script); err != nil {
		t.Fatalf("RunRemoteScript error: %v", err)
	}

	cmds := server.Commands()
	if len(cmds) != 1 || cmds[0] != script {
		t.Fatalf("expected command %q, got %v", script, cmds)
	}

	logs := observed.All()
	if len(logs) != 1 {
		t.Fatalf("expected one log entry, got %d", len(logs))
	}
	if logs[0].Message != "Running remote script" {
		t.Fatalf("unexpected log message %q", logs[0].Message)
	}
	if logs[0].ContextMap()["script"] != script {
		t.Fatalf("expected script %q in log, got %v", script, logs[0].ContextMap()["script"])
	}
}
