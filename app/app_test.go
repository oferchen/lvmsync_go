package app

import (
	"context"
	"os"
	signalpkg "os/signal"
	"syscall"
	"testing"
	"time"

	"go.uber.org/zap"

	signalspkg "lvmsync_go/cmd/lvmsync/signals"
	"lvmsync_go/internal/config"
)

// Test SetupSignalHandling registers OS signals and spawns the handler goroutine.
func TestSetupSignalHandling(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg := &config.Config{}
	logger := zap.NewNop()
	snapshotPath := ""

	called := make(chan struct{})
	fakeHandler := signalspkg.HandlerFunc(func(_ context.Context, _ *config.Config, _ *zap.Logger, _ <-chan os.Signal, _ *string, _ chan<- error) {
		close(called)
		<-ctx.Done()
	})

	r := NewRunnerWithDeps(&Runner{signalsHandler: fakeHandler})
	signalsCh, errCh := r.SetupSignalHandling(ctx, cfg, &snapshotPath, logger)

	// ensure handler goroutine started
	select {
	case <-called:
	case <-time.After(time.Second):
		t.Fatalf("handler not invoked")
	}

	// send SIGTERM to ourselves and expect it on the registered channel
	if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatalf("failed to send SIGTERM: %v", err)
	}
	select {
	case sig := <-signalsCh:
		if sig != syscall.SIGTERM {
			t.Fatalf("expected SIGTERM, got %v", sig)
		}
	case <-time.After(time.Second):
		t.Fatalf("timeout waiting for signal")
	}

	if errCh == nil {
		t.Fatalf("expected non-nil error channel")
	}

	signalpkg.Stop(signalsCh)
}

// Test PrepareSnapshot passes arguments through to the injected function and returns its values.
func TestPrepareSnapshot(t *testing.T) {
	ctx := context.Background()
	cfg := &config.Config{}
	logger := zap.NewNop()
	original := "vol1"

	expectedPath := "snap0"
	expectedErrCh := make(chan error)
	cleanupCalled := false

	var gotCtx context.Context
	var gotCfg *config.Config
	var gotVol string
	var gotLogger *zap.Logger

	fakePrepare := func(ctx context.Context, cfg *config.Config, vol string, l *zap.Logger) (string, chan error, func(), error) {
		gotCtx = ctx
		gotCfg = cfg
		gotVol = vol
		gotLogger = l
		return expectedPath, expectedErrCh, func() { cleanupCalled = true }, nil
	}

	r := NewRunnerWithDeps(&Runner{prepareSnapshot: fakePrepare})
	path, errCh, cleanup, err := r.PrepareSnapshot(ctx, cfg, original, logger)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if path != expectedPath {
		t.Fatalf("expected path %q, got %q", expectedPath, path)
	}
	if errCh != expectedErrCh {
		t.Fatalf("unexpected error channel")
	}

	cleanup()
	if !cleanupCalled {
		t.Fatalf("expected cleanup to be invoked")
	}

	if gotCtx != ctx || gotCfg != cfg || gotVol != original || gotLogger != logger {
		t.Fatalf("arguments not passed through")
	}
}
