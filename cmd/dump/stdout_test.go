package dump

import (
	"context"
	"io"
	"os"
	"testing"
	"time"

	"go.uber.org/zap"

	"lvmsync_go/device"
	"lvmsync_go/internal/config"
	"lvmsync_go/transfer"
)

func TestRunStdout(t *testing.T) {
	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig returned error: %v", err)
	}
	cfg.StdoutMode = true
	cfg.DedupStrategy = "none"
	cfg.Parallel = 1
	cfg.SpeedLimit = 0

	expected := "test output"

	originalFunc := dumpChangesSequential
	dumpChangesSequential = func(_ context.Context, _ *transfer.Transfer, c *config.Config, snapshot, source string, out io.Writer) error {
		_, writeErr := out.Write([]byte(expected))
		return writeErr
	}
	defer func() { dumpChangesSequential = originalFunc }()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	oldStdout := os.Stdout
	os.Stdout = w

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	origDetect := detectDevice
	detectDevice = func(context.Context, string, bool, string, string, string, string, time.Duration, time.Duration, *zap.Logger, *device.Runner) (device.Device, error) {
		return &fakeDevice{path: "/dev/snap"}, nil
	}
	defer func() { detectDevice = origDetect }()
	if _, err = Run(ctx, cfg, "/dev/snap", "", zap.NewNop()); err != nil {
		t.Fatalf("Run returned error: %v", err)
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
