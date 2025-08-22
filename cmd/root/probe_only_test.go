package root

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"go.uber.org/zap"

	"lvmsync_go/internal/config"
	"lvmsync_go/transport"
)

func TestRunProbeOnlyOutputsIdentityTuple(t *testing.T) {
	// Probe-only output format:
	// size_bytes kernel_uuid gpt_uuid mbr_signature fs_uuid major minor manifest_epoch
	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	cfg.ProbeOnly = true

	src := t.TempDir() + "/src"
	if err := os.WriteFile(src, []byte("data"), 0o600); err != nil {
		t.Fatalf("write src: %v", err)
	}
	dst := t.TempDir() + "/dst"

	fi, err := os.Stat(src)
	if err != nil {
		t.Fatalf("stat src: %v", err)
	}

	r := NewRunnerWithDeps(&Runner{
		selectTransportFn: func(*config.Config, *zap.Logger) (transport.Interface, error) { return nil, nil },
		setupSignalHandleFn: func(ctx context.Context, cfg *config.Config, snap *string, _ *zap.Logger) (chan os.Signal, chan error) {
			return make(chan os.Signal), make(chan error)
		},
		prepareSnapshotFn: func(context.Context, *config.Config, string, *zap.Logger) (string, chan error, func(), error) {
			return src, make(chan error), func() {}, nil
		},
		executeClientFn: func(ctx context.Context, runClient func(context.Context, string, string) error, snap, dest string, _ chan error, _ chan error, _ *zap.Logger) error {
			return runClient(ctx, snap, dest)
		},
		runDumpFn: func(ctx context.Context, cfg *config.Config, snapshot, dest string, logger *zap.Logger) (string, error) {
			_, err := fmt.Fprintf(os.Stdout, "%d k g  f 1 2 3\n", fi.Size())
			return "", err
		},
	})

	oldStdout := os.Stdout
	rPipe, wPipe, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = wPipe

	if err := r.Run(cfg, []string{src, dst}, zap.NewNop()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	wPipe.Close()
	os.Stdout = oldStdout
	out, err := io.ReadAll(rPipe)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	got := strings.TrimSpace(string(out))
	exp := fmt.Sprintf("%d k g  f 1 2 3", fi.Size())
	if got != exp {
		t.Fatalf("expected %q, got %q", exp, got)
	}
	parts := strings.Split(got, " ")
	if len(parts) != 8 {
		t.Fatalf("expected 8 fields, got %d: %q", len(parts), got)
	}
}
