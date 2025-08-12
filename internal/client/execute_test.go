package client

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestExecuteRunError(t *testing.T) {
	runClient := func(context.Context, string, string) error { return errors.New("run") }
	sigCh := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err := ExecuteClient(ctx, runClient, "snap", "dest", sigCh, nil)
	if err == nil || !strings.Contains(err.Error(), "copy operation failed") {
		t.Fatalf("expected copy failure, got %v", err)
	}
}

func TestExecuteSignal(t *testing.T) {
	block := make(chan struct{})
	runClient := func(context.Context, string, string) error {
		<-block
		return nil
	}
	sigCh := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		time.Sleep(10 * time.Millisecond)
		sigCh <- errors.New("signal")
	}()
	err := ExecuteClient(ctx, runClient, "snap", "dest", sigCh, nil)
	close(block)
	if err == nil || !strings.Contains(err.Error(), "signal") {
		t.Fatalf("expected signal error, got %v", err)
	}
}

func TestExecuteMonitorError(t *testing.T) {
	runClient := func(context.Context, string, string) error { return nil }
	sigCh := make(chan error, 1)
	monitorCh := make(chan error, 1)
	monitorCh <- errors.New("monitor")
	close(monitorCh)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err := ExecuteClient(ctx, runClient, "snap", "dest", sigCh, monitorCh)
	if err == nil || !strings.Contains(err.Error(), "snapshot monitor error") {
		t.Fatalf("expected monitor error, got %v", err)
	}
}
