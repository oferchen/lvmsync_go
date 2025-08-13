package root

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"lvmsync_go/config"

	"go.uber.org/goleak"
	"go.uber.org/zap"
)

func TestSetupGRPCSuccess(t *testing.T) {
	cfg := &config.Config{}
	logger := zap.NewNop()
	srvCleanCalled := false
	clientCleanCalled := false

	origStart := startGRPCServer
	origClient := clientHandshake
	defer func() {
		startGRPCServer = origStart
		clientHandshake = origClient
	}()

	srvErrCh := make(chan error)
	startGRPCServer = func(_ context.Context, _ *config.Config, _ *zap.Logger) (func(), <-chan error, error) {
		return func() { srvCleanCalled = true }, srvErrCh, nil
	}
	clientHandshake = func(_ *config.Config, _ *zap.Logger) (func(), chan error, error) {
		return func() { clientCleanCalled = true }, make(chan error), nil
	}

	cleanupSrv, cleanupClient, errCh, err := SetupGRPC(context.Background(), cfg, logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	done := make(chan struct{})
	go func() {
		cleanupSrv()
		close(done)
	}()
	select {
	case <-done:
		t.Fatalf("cleanupSrv returned before errCh signal")
	case <-time.After(10 * time.Millisecond):
	}
	srvErrCh <- nil
	<-done
	cleanupClient()
	if errCh == nil {
		t.Fatalf("expected error channel")
	}
	if !srvCleanCalled || !clientCleanCalled {
		t.Fatalf("cleanup functions not called")
	}
}

func TestSetupGRPCServerError(t *testing.T) {
	cfg := &config.Config{}
	logger := zap.NewNop()
	origStart := startGRPCServer
	defer func() { startGRPCServer = origStart }()
	startGRPCServer = func(_ context.Context, _ *config.Config, _ *zap.Logger) (func(), <-chan error, error) {
		return nil, nil, errors.New("srv fail")
	}
	if _, _, _, err := SetupGRPC(context.Background(), cfg, logger); err == nil {
		t.Fatalf("expected error")
	}
}

func TestSetupGRPCClientError(t *testing.T) {
	cfg := &config.Config{}
	logger := zap.NewNop()
	srvCleanupCalled := false
	origStart := startGRPCServer
	origClient := clientHandshake
	defer func() {
		startGRPCServer = origStart
		clientHandshake = origClient
	}()
	srvErrCh := make(chan error)
	startGRPCServer = func(_ context.Context, _ *config.Config, _ *zap.Logger) (func(), <-chan error, error) {
		return func() { srvCleanupCalled = true }, srvErrCh, nil
	}
	clientHandshake = func(_ *config.Config, _ *zap.Logger) (func(), chan error, error) {
		return nil, nil, errors.New("client fail")
	}
	done := make(chan struct{})
	go func() {
		_, _, _, err := SetupGRPC(context.Background(), cfg, logger)
		if err == nil {
			t.Errorf("expected error")
		}
		close(done)
	}()
	select {
	case <-done:
		t.Fatalf("SetupGRPC returned before errCh signal")
	case <-time.After(10 * time.Millisecond):
	}
	srvErrCh <- nil
	<-done
	if !srvCleanupCalled {
		t.Fatalf("server cleanup not called on error")
	}
}

func TestSetupGRPCServeFailurePropagation(t *testing.T) {
	cfg := &config.Config{}
	logger := zap.NewNop()
	srvCleanupCalled := false
	origStart := startGRPCServer
	origClient := clientHandshake
	defer func() {
		startGRPCServer = origStart
		clientHandshake = origClient
	}()
	startGRPCServer = func(_ context.Context, _ *config.Config, _ *zap.Logger) (func(), <-chan error, error) {
		errCh := make(chan error, 1)
		errCh <- errors.New("serve boom")
		return func() { srvCleanupCalled = true; close(errCh) }, errCh, nil
	}
	clientHandshake = func(_ *config.Config, _ *zap.Logger) (func(), chan error, error) {
		t.Fatalf("client handshake should not be called")
		return nil, nil, nil
	}
	if _, _, _, err := SetupGRPC(context.Background(), cfg, logger); err == nil {
		t.Fatalf("expected error from serve failure")
	}
	if !srvCleanupCalled {
		t.Fatalf("server cleanup not called on serve failure")
	}
}

func TestPrepareSnapshot(t *testing.T) {
	cfg := &config.Config{}
	logger := zap.NewNop()
	orig := prepareSnapshotFn
	defer func() { prepareSnapshotFn = orig }()
	prepareSnapshotFn = func(ctx context.Context, c *config.Config, v string, l *zap.Logger) (string, chan error, func(), error) {
		return "snap", nil, func() {}, nil
	}
	snap, ch, cleanup, err := PrepareSnapshot(context.Background(), cfg, "vol", logger)
	if err != nil || snap != "snap" || ch != nil || cleanup == nil {
		t.Fatalf("unexpected result")
	}
}

func TestExecuteClient(t *testing.T) {
	cfg := &config.Config{}
	logger := zap.NewNop()
	called := false
	origExec := executeClientFn
	origRunDump := runDump
	defer func() {
		executeClientFn = origExec
		runDump = origRunDump
	}()
	executeClientFn = func(ctx context.Context, f func(context.Context, string, string) error, snap, dest string, sigErrCh, monitorErrCh chan error) error {
		called = true
		if snap != "s" || dest != "d" {
			t.Fatalf("unexpected args")
		}
		return f(ctx, snap, dest)
	}
	runDump = func(_ context.Context, _ *config.Config, snap, dest string, _ *zap.Logger) error {
		if snap != "s" || dest != "d" {
			t.Fatalf("unexpected dump args")
		}
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := ExecuteClient(ctx, cfg, "s", "d", nil, nil, logger); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatalf("executeClientFn not called")
	}
}

func TestRunHeartbeatError(t *testing.T) {
	cfg := &config.Config{StdoutMode: true}
	logger := zap.NewNop()

	origStart := startGRPCServer
	origClient := clientHandshake
	origSetup := setupSignalHandle
	origPrepare := prepareSnapshotFn
	origExec := executeClientFn
	origSelect := selectTransport
	defer func() {
		startGRPCServer = origStart
		clientHandshake = origClient
		setupSignalHandle = origSetup
		prepareSnapshotFn = origPrepare
		executeClientFn = origExec
		selectTransport = origSelect
	}()

	hbErrCh := make(chan error, 1)
	hbErrCh <- errors.New("hb fail")

	startGRPCServer = func(context.Context, *config.Config, *zap.Logger) (func(), <-chan error, error) {
		ch := make(chan error)
		close(ch)
		return func() {}, ch, nil
	}
	clientHandshake = func(*config.Config, *zap.Logger) (func(), chan error, error) {
		return func() {}, hbErrCh, nil
	}
	selectTransport = func(*config.Config, *zap.Logger) error { return nil }

	sigErrCh := make(chan error, 1)
	setupSignalHandle = func(_ context.Context, _ *config.Config, _ *string, _ *zap.Logger) (chan os.Signal, chan error) {
		return nil, sigErrCh
	}
	prepareSnapshotFn = func(ctx context.Context, _ *config.Config, _ string, _ *zap.Logger) (string, chan error, func(), error) {
		return "snap", nil, func() {}, nil
	}
	executeClientFn = func(ctx context.Context, f func(context.Context, string, string) error, snap, dest string, sigErrCh, monitorErrCh chan error) error {
		return <-sigErrCh
	}

	err := Run(cfg, []string{"vol"}, logger)
	if err == nil || err.Error() != "hb fail" {
		t.Fatalf("expected heartbeat error, got %v", err)
	}
}

func TestRunGRPCConnectGoroutineLeak(t *testing.T) {
	cases := []struct {
		name        string
		grpcConnect string
		hbChannel   bool
	}{
		{name: "no_grpc_connect", grpcConnect: "", hbChannel: false},
		{name: "with_grpc_connect", grpcConnect: "addr", hbChannel: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer goleak.VerifyNone(t)

			cfg := &config.Config{StdoutMode: true, GRPCConnect: tc.grpcConnect}
			logger := zap.NewNop()

			origStart := startGRPCServer
			origClient := clientHandshake
			origSetup := setupSignalHandle
			origPrepare := prepareSnapshotFn
			origExec := executeClientFn
			origSelect := selectTransport
			defer func() {
				startGRPCServer = origStart
				clientHandshake = origClient
				setupSignalHandle = origSetup
				prepareSnapshotFn = origPrepare
				executeClientFn = origExec
				selectTransport = origSelect
			}()

			startGRPCServer = func(context.Context, *config.Config, *zap.Logger) (func(), <-chan error, error) {
				ch := make(chan error)
				close(ch)
				return func() {}, ch, nil
			}

			if tc.hbChannel {
				hbErrCh := make(chan error)
				clientHandshake = func(*config.Config, *zap.Logger) (func(), chan error, error) {
					return func() { close(hbErrCh) }, hbErrCh, nil
				}
			} else {
				clientHandshake = func(*config.Config, *zap.Logger) (func(), chan error, error) {
					return func() {}, nil, nil
				}
			}

			setupSignalHandle = func(context.Context, *config.Config, *string, *zap.Logger) (chan os.Signal, chan error) {
				return nil, make(chan error, 1)
			}

			selectTransport = func(*config.Config, *zap.Logger) error { return nil }

			prepareSnapshotFn = func(context.Context, *config.Config, string, *zap.Logger) (string, chan error, func(), error) {
				return "snap", nil, func() {}, nil
			}

			executeClientFn = func(context.Context, func(context.Context, string, string) error, string, string, chan error, chan error) error {
				return nil
			}

			if err := Run(cfg, []string{"vol"}, logger); err != nil {
				t.Fatalf("Run returned error: %v", err)
			}

			time.Sleep(10 * time.Millisecond)
		})
	}
}
