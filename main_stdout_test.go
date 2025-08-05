package main

import (
	"io"
	"os"
	"testing"

	"lvmsync_go/config"
)

func TestRunClientModeStdout(t *testing.T) {
	cfg = config.DefaultConfig()
	cfg.StdoutMode = true
	cfg.DedupStrategy = "none"
	cfg.Parallel = 1
	cfg.SpeedLimit = 0

	expected := "test output"

	originalFunc := dumpChangesSequential
	dumpChangesSequential = func(c *config.Config, snapshot, source string, out io.Writer) error {
		_, err := out.Write([]byte(expected))
		return err
	}
	defer func() { dumpChangesSequential = originalFunc }()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	oldStdout := os.Stdout
	os.Stdout = w

	if err := runClientMode("/dev/snap", ""); err != nil {
		t.Fatalf("runClientMode returned error: %v", err)
	}

	w.Close()
	os.Stdout = oldStdout
	output, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("failed to read stdout: %v", err)
	}

	if string(output) != expected {
		t.Fatalf("expected %q, got %q", expected, string(output))
	}
}
