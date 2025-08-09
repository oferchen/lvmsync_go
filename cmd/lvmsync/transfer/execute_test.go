package transfer

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestExecuteRunError(t *testing.T) {
	runClient := func(string, string) error { return errors.New("run") }
	sigCh := make(chan error, 1)
	err := Execute(runClient, "snap", "dest", sigCh, nil)
	if err == nil || !strings.Contains(err.Error(), "copy operation failed") {
		t.Fatalf("expected copy failure, got %v", err)
	}
}

func TestExecuteSignal(t *testing.T) {
	block := make(chan struct{})
	runClient := func(string, string) error {
		<-block
		return nil
	}
	sigCh := make(chan error, 1)
	go func() {
		time.Sleep(10 * time.Millisecond)
		sigCh <- errors.New("signal")
	}()
	err := Execute(runClient, "snap", "dest", sigCh, nil)
	close(block)
	if err == nil || !strings.Contains(err.Error(), "signal") {
		t.Fatalf("expected signal error, got %v", err)
	}
}

func TestExecuteMonitorError(t *testing.T) {
	runClient := func(string, string) error { return nil }
	sigCh := make(chan error, 1)
	monitorCh := make(chan error, 1)
	monitorCh <- errors.New("monitor")
	err := Execute(runClient, "snap", "dest", sigCh, monitorCh)
	if err == nil || !strings.Contains(err.Error(), "snapshot monitor error") {
		t.Fatalf("expected monitor error, got %v", err)
	}
}
