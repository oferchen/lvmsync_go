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
	"lvmsync_go/internal/privilege"
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
	r := NewRunnerWithDeps(&Runner{
		dumpSeq: func(_ context.Context, _ *transfer.Transfer, c *config.Config, snapshot, source string, out io.Writer) error {
			_, writeErr := out.Write([]byte(expected))
			return writeErr
		},
		detectDevice: func(context.Context, string, bool, string, string, string, string, time.Duration, time.Duration, privilege.Escalator, *zap.Logger, *device.Runner) (device.Device, error) {
			return &fakeDevice{path: "/dev/snap"}, nil
		},
		verifyIdentity: func(context.Context, *device.Info, string, string) error { return nil },
	})

	rPipe, wPipe, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	oldStdout := os.Stdout
	os.Stdout = wPipe

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err = r.Run(ctx, cfg, "/dev/snap", "", zap.NewNop()); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	wPipe.Close()
	os.Stdout = oldStdout
	output, err := io.ReadAll(rPipe)
	if err != nil {
		t.Fatalf("failed to read stdout: %v", err)
	}

	if string(output) != expected {
		t.Fatalf("expected %q, got %q", expected, string(output))
	}
}
