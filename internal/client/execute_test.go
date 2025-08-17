package client

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestExecuteRunError(t *testing.T) {
	runClient := func(context.Context, string, string) error { return errors.New("run") }
	sigCh := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	logger := zap.NewNop()
	err := ExecuteClient(ctx, runClient, "snap", "dest", sigCh, nil, logger)
	if err == nil || err.Error() != "run" {
		t.Fatalf("expected run error, got %v", err)
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
	logger := zap.NewNop()
	err := ExecuteClient(ctx, runClient, "snap", "dest", sigCh, nil, logger)
	close(block)
	if err == nil || err.Error() != "signal" {
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
	logger := zap.NewNop()
	err := ExecuteClient(ctx, runClient, "snap", "dest", sigCh, monitorCh, logger)
	if err == nil || err.Error() != "monitor" {
		t.Fatalf("expected monitor error, got %v", err)
	}
}
