package remote

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestValidateRemoteCommand(t *testing.T) {
	_, rawClient := newSSHServerClient(t, func(cmd string) int {
		switch cmd {
		case "echo --version":
			return 0
		case "nonexistent --version":
			return 127
		default:
			return 0
		}
	})

	client := &SSHClient{Client: rawClient, Logger: zap.NewNop()}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := client.ValidateRemoteCommand(ctx, "echo"); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	ctx2, cancel2 := context.WithTimeout(context.Background(), time.Second)
	defer cancel2()
	if err := client.ValidateRemoteCommand(ctx2, "nonexistent"); err == nil {
		t.Fatalf("expected error for nonexistent command")
	}
}

func TestRunRemoteScript(t *testing.T) {
	server, rawClient := newSSHServerClient(t, func(_ string) int { return 0 }) // cmd is unused

	core, observed := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	defer logger.Sync()

	client := &SSHClient{Client: rawClient, Logger: logger}

	script := "echo hi"
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := client.RunRemoteScript(ctx, script); err != nil {
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

func TestRunRemoteScriptCanceled(t *testing.T) {
	_, rawClient := newSSHServerClient(t, func(cmd string) int {
		time.Sleep(100 * time.Millisecond)
		return 0
	})
	client := &SSHClient{Client: rawClient, Logger: zap.NewNop()}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := client.RunRemoteScript(ctx, "echo hi"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
}

func TestValidateRemoteCommandCanceled(t *testing.T) {
	_, rawClient := newSSHServerClient(t, func(cmd string) int {
		time.Sleep(100 * time.Millisecond)
		return 0
	})
	client := &SSHClient{Client: rawClient, Logger: zap.NewNop()}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := client.ValidateRemoteCommand(ctx, "echo"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
}

func TestValidateRemoteCommandInvalidChars(t *testing.T) {
	client := &SSHClient{Logger: zap.NewNop()}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := client.ValidateRemoteCommand(ctx, "bad+cmd"); err == nil || !strings.Contains(err.Error(), "invalid characters") {
		t.Fatalf("expected invalid characters error, got %v", err)
	}
}
