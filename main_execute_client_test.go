package main

import (
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"lvmsync_go/config"
)

func setupExecuteClientTest(t *testing.T) {
	t.Helper()
	var err error
	cfg, err = config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig returned error: %v", err)
	}
	cfg.StdoutMode = true
	cfg.DedupStrategy = "none"
	cfg.Parallel = 1
	cfg.SpeedLimit = 0
}

func TestExecuteClientRunError(t *testing.T) {
	setupExecuteClientTest(t)
	orig := dumpChangesSequential
	dumpChangesSequential = func(c *config.Config, snap, origin string, out io.Writer) error {
		return errors.New("dump")
	}
	defer func() { dumpChangesSequential = orig }()

	sigCh := make(chan error, 1)
	err := executeClient("snap", "dest", sigCh, nil)
	if err == nil || !strings.Contains(err.Error(), "copy operation failed") {
		t.Fatalf("expected copy failure, got %v", err)
	}
}

func TestExecuteClientSignal(t *testing.T) {
	setupExecuteClientTest(t)
	block := make(chan struct{})
	orig := dumpChangesSequential
	dumpChangesSequential = func(c *config.Config, snap, origin string, out io.Writer) error {
		<-block
		return nil
	}
	defer func() { dumpChangesSequential = orig }()

	sigCh := make(chan error, 1)
	go func() {
		time.Sleep(10 * time.Millisecond)
		sigCh <- errors.New("signal")
	}()
	err := executeClient("snap", "dest", sigCh, nil)
	close(block)
	if err == nil || !strings.Contains(err.Error(), "signal") {
		t.Fatalf("expected signal error, got %v", err)
	}
}

func TestExecuteClientMonitorError(t *testing.T) {
	setupExecuteClientTest(t)
	orig := dumpChangesSequential
	dumpChangesSequential = func(c *config.Config, snap, origin string, out io.Writer) error {
		return nil
	}
	defer func() { dumpChangesSequential = orig }()

	sigCh := make(chan error, 1)
	monitorCh := make(chan error, 1)
	monitorCh <- errors.New("monitor")
	err := executeClient("snap", "dest", sigCh, monitorCh)
	if err == nil || !strings.Contains(err.Error(), "snapshot monitor error") {
		t.Fatalf("expected monitor error, got %v", err)
	}
}
