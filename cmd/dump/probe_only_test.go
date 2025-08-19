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
)

func TestProbeOnlyNoSideEffects(t *testing.T) {
	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig returned error: %v", err)
	}
	cfg.ProbeOnly = true

	r := NewRunnerWithDeps(&Runner{
		detectDevice: func(context.Context, string, bool, string, string, string, string, time.Duration, time.Duration, privilege.Escalator, *zap.Logger, *device.Runner) (device.Device, error) {
			t.Fatalf("detectDevice called")
			return nil, nil
		},
		openFile: func(string, int, os.FileMode) (*os.File, error) {
			t.Fatalf("openFile called")
			return nil, nil
		},
		probeDest: func(context.Context, *config.Config, string, *zap.Logger) (device.DeviceIdentity, error) {
			return device.DeviceIdentity{SizeBytes: 123, KernelUUID: "k", GPTUUID: "g", FSUUID: "f", Major: 1, Minor: 2, ManifestEpoch: 456}, nil
		},
	})

	oldStdout := os.Stdout
	rPipe, wPipe, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe: %v", err)
	}
	os.Stdout = wPipe

	if _, err := r.Run(context.Background(), cfg, "src", "dest", zap.NewNop()); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	wPipe.Close()
	os.Stdout = oldStdout
	out, err := io.ReadAll(rPipe)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	expected := "123 k g f 1 2 456\n"
	if string(out) != expected {
		t.Fatalf("expected %q, got %q", expected, string(out))
	}
}
