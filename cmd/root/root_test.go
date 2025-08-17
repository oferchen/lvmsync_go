package root

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"testing"
	"time"

	"lvmsync_go/internal/config"
	privilege "lvmsync_go/internal/privilege"
	"lvmsync_go/transport"

	"go.uber.org/goleak"
	"go.uber.org/zap"
)

func TestDispatchSubcommandManifest(t *testing.T) {
	cfg := &config.Config{}
	logger := zap.NewNop()
	r := NewRunner()
	called := false
	r.runManifestFn = func(c *config.Config, args []string, l *zap.Logger) error {
		called = true
		if args[0] != "arg" {
			t.Fatalf("unexpected arg")
		}
		return nil
	}
	handled, err := r.dispatchSubcommand(cfg, []string{"manifest", "arg"}, logger)
	if err != nil || !handled || !called {
		t.Fatalf("dispatch failed")
	}
}

func TestDispatchSubcommandApplyError(t *testing.T) {
	cfg := &config.Config{ApplyMode: "apply"}
	logger := zap.NewNop()
	r := NewRunner()
	r.runApplyFn = func(*config.Config, string, []string, *zap.Logger) error { return errors.New("boom") }
	handled, err := r.dispatchSubcommand(cfg, []string{"vol"}, logger)
	if !handled || err == nil || err.Error() != "apply operation failed: boom" {
		t.Fatalf("expected wrapped apply error, got %v", err)
	}
}

func TestPrepareClientSuccess(t *testing.T) {
	cfg := &config.Config{StdoutMode: true, GRPCSetupTimeout: time.Second, SourceType: "file"}
	logger := zap.NewNop()
	r := NewRunner()
	selectCalled := false
	srvClean := false
	clientClean := false
	r.selectTransportFn = func(*config.Config, *zap.Logger) (transport.Interface, error) {
		selectCalled = true
		return nil, nil
	}
	r.startGRPCServerFn = func(context.Context, *config.Config, *zap.Logger) (func(), <-chan error, error) {
		ch := make(chan error, 1)
		go func() {
			ch <- nil
			close(ch)
		}()
		return func() { srvClean = true }, ch, nil
	}
	r.clientHandshakeFn = func(context.Context, *config.Config, *zap.Logger) (func(), chan error, error) {
		return func() { clientClean = true }, nil, nil
	}
	r.setupSignalHandleFn = func(context.Context, *config.Config, *string, *zap.Logger) (chan os.Signal, chan error) {
		return make(chan os.Signal), make(chan error)
	}
	ctx, cleanup, snap, dest, sigErrCh, err := r.prepareClient(cfg, []string{"vol"}, logger)
	if err != nil || ctx == nil || cleanup == nil || snap != "vol" || dest != "" || sigErrCh == nil || !selectCalled {
		t.Fatalf("prepareClient failed: %v", err)
	}
	cleanup()
	if !srvClean || !clientClean {
		t.Fatalf("cleanup not called")
	}
}

func TestPrepareClientSelectTransportError(t *testing.T) {
	cfg := &config.Config{StdoutMode: true, GRPCSetupTimeout: time.Second, SourceType: "file"}
	logger := zap.NewNop()
	r := NewRunner()
	r.selectTransportFn = func(*config.Config, *zap.Logger) (transport.Interface, error) {
		return nil, errors.New("bad")
	}
	_, _, _, _, _, err := r.prepareClient(cfg, []string{"vol"}, logger)
	if err == nil || err.Error() != "select transport: bad" {
		t.Fatalf("expected select transport error, got %v", err)
	}
}

func TestExecuteSync(t *testing.T) {
	cfg := &config.Config{}
	logger := zap.NewNop()
	called := false
	r := NewRunner()
	r.executeClientFn = func(context.Context, func(context.Context, string, string) error, string, string, chan error, chan error, *zap.Logger) error {
		called = true
		return nil
	}
	if err := r.executeSync(context.Background(), cfg, "s", "d", nil, logger); err != nil || !called {
		t.Fatalf("executeSync failed: %v", err)
	}
	r.executeClientFn = func(context.Context, func(context.Context, string, string) error, string, string, chan error, chan error, *zap.Logger) error {
		return errors.New("x")
	}
	if err := r.executeSync(context.Background(), cfg, "s", "d", nil, logger); err == nil || err.Error() != "x" {
		t.Fatalf("expected error")
	}
}

func TestSetupGRPCSuccess(t *testing.T) {
	cfg := &config.Config{SourceType: "file"}
	logger := zap.NewNop()
	srvCleanCalled := false
	clientCleanCalled := false
	r := NewRunner()
	srvErrCh := make(chan error)
	r.startGRPCServerFn = func(_ context.Context, _ *config.Config, _ *zap.Logger) (func(), <-chan error, error) {
		return func() { srvCleanCalled = true }, srvErrCh, nil
	}
	r.clientHandshakeFn = func(_ context.Context, _ *config.Config, _ *zap.Logger) (func(), chan error, error) {
		return func() { clientCleanCalled = true }, make(chan error), nil
	}

	cleanupSrv, cleanupClient, errCh, err := r.SetupGRPC(context.Background(), cfg, logger)
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

func TestConfigureLogsBlockSizeBytes(t *testing.T) {
	origArgs := os.Args
	os.Args = []string{origArgs[0], "--block_size", "4KB"}
	defer func() { os.Args = origArgs }()

	origCaps := privilege.HasCaps
	privilege.HasCaps = func() bool { return true }
	defer func() { privilege.HasCaps = origCaps }()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	origStderr := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = origStderr }()

	cfg, _, logger, err := Configure()
	if err != nil {
		t.Fatalf("Configure error: %v", err)
	}

	SyncLogger(logger)
	w.Close()
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	scan := bufio.NewScanner(bytes.NewReader(data))
	found := false
	for scan.Scan() {
		var m map[string]any
		if err := json.Unmarshal(scan.Bytes(), &m); err != nil {
			continue
		}
		if m["msg"] == "Effective configuration" {
			v, ok := m["block_size_bytes"].(float64)
			if !ok || uint64(v) != cfg.BlockSizeBytes() {
				t.Fatalf("expected block_size_bytes %d, got %v", cfg.BlockSizeBytes(), m["block_size_bytes"])
			}
			if _, ok := m["block_size"].(string); !ok {
				t.Fatalf("missing block_size")
			}
			found = true
			break
		}
	}
	if err := scan.Err(); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if !found {
		t.Fatalf("Effective configuration log not found")
	}
}

func TestSetupGRPCServerError(t *testing.T) {
	cfg := &config.Config{SourceType: "file"}
	logger := zap.NewNop()
	r := NewRunner()
	r.startGRPCServerFn = func(_ context.Context, _ *config.Config, _ *zap.Logger) (func(), <-chan error, error) {
		return nil, nil, errors.New("srv fail")
	}
	if _, _, _, err := r.SetupGRPC(context.Background(), cfg, logger); err == nil {
		t.Fatalf("expected error")
	}
}

func TestSetupGRPCClientError(t *testing.T) {
	cfg := &config.Config{SourceType: "file"}
	logger := zap.NewNop()
	srvCleanupCalled := false
	r := NewRunner()
	srvErrCh := make(chan error)
	r.startGRPCServerFn = func(_ context.Context, _ *config.Config, _ *zap.Logger) (func(), <-chan error, error) {
		return func() { srvCleanupCalled = true }, srvErrCh, nil
	}
	r.clientHandshakeFn = func(_ context.Context, _ *config.Config, _ *zap.Logger) (func(), chan error, error) {
		return nil, nil, errors.New("client fail")
	}
	done := make(chan struct{})
	go func() {
		_, _, _, err := r.SetupGRPC(context.Background(), cfg, logger)
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

func TestSetupGRPCTimeout(t *testing.T) {
	cfg := &config.Config{}
	logger := zap.NewNop()
	srvCleanupCalled := false
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	r := NewRunner()
	r.startGRPCServerFn = func(ctx context.Context, _ *config.Config, _ *zap.Logger) (func(), <-chan error, error) {
		ch := make(chan error, 1)
		go func() { <-ctx.Done(); ch <- ctx.Err() }()
		return func() { srvCleanupCalled = true }, ch, nil
	}
	r.clientHandshakeFn = func(ctx context.Context, _ *config.Config, _ *zap.Logger) (func(), chan error, error) {
		<-ctx.Done()
		return func() {}, nil, ctx.Err()
	}
	if _, _, _, err := r.SetupGRPC(ctx, cfg, logger); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
	if !srvCleanupCalled {
		t.Fatalf("server cleanup not called on timeout")
	}
}

func TestSetupGRPCServeFailurePropagation(t *testing.T) {
	cfg := &config.Config{}
	logger := zap.NewNop()
	srvCleanupCalled := false
	r := NewRunner()
	r.startGRPCServerFn = func(_ context.Context, _ *config.Config, _ *zap.Logger) (func(), <-chan error, error) {
		errCh := make(chan error, 1)
		errCh <- errors.New("serve boom")
		return func() { srvCleanupCalled = true; close(errCh) }, errCh, nil
	}
	r.clientHandshakeFn = func(_ context.Context, _ *config.Config, _ *zap.Logger) (func(), chan error, error) {
		t.Fatalf("client handshake should not be called")
		return nil, nil, nil
	}
	if _, _, _, err := r.SetupGRPC(context.Background(), cfg, logger); err == nil {
		t.Fatalf("expected error from serve failure")
	}
	if !srvCleanupCalled {
		t.Fatalf("server cleanup not called on serve failure")
	}
}

func TestPrepareSnapshot(t *testing.T) {
	cfg := &config.Config{}
	logger := zap.NewNop()
	r := NewRunner()
	r.prepareSnapshotFn = func(ctx context.Context, c *config.Config, v string, l *zap.Logger) (string, chan error, func(), error) {
		return "snap", nil, func() {}, nil
	}
	snap, ch, cleanup, err := r.PrepareSnapshot(context.Background(), cfg, "vol", logger)
	if err != nil || snap != "snap" || ch != nil || cleanup == nil {
		t.Fatalf("unexpected result")
	}
}

func TestExecuteClient(t *testing.T) {
	cfg := &config.Config{}
	logger := zap.NewNop()
	called := false
	r := NewRunner()
	r.executeClientFn = func(ctx context.Context, f func(context.Context, string, string) error, snap, dest string, sigErrCh, monitorErrCh chan error, _ *zap.Logger) error {
		called = true
		if snap != "s" || dest != "d" {
			t.Fatalf("unexpected args")
		}
		return f(ctx, snap, dest)
	}
	r.runDumpFn = func(_ context.Context, _ *config.Config, snap, dest string, _ *zap.Logger) (string, error) {
		if snap != "s" || dest != "d" {
			t.Fatalf("unexpected dump args")
		}
		return "", nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := r.ExecuteClient(ctx, cfg, "s", "d", nil, nil, logger); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatalf("executeClientFn not called")
	}
}

func TestRunHeartbeatError(t *testing.T) {
	cfg := &config.Config{StdoutMode: true, GRPCSetupTimeout: time.Second, SourceType: "file"}
	logger := zap.NewNop()
	r := NewRunner()
	hbErrCh := make(chan error, 1)
	hbErrCh <- errors.New("hb fail")
	r.startGRPCServerFn = func(context.Context, *config.Config, *zap.Logger) (func(), <-chan error, error) {
		ch := make(chan error)
		close(ch)
		return func() {}, ch, nil
	}
	r.clientHandshakeFn = func(context.Context, *config.Config, *zap.Logger) (func(), chan error, error) {
		return func() {}, hbErrCh, nil
	}
	r.selectTransportFn = func(*config.Config, *zap.Logger) (transport.Interface, error) { return nil, nil }
	sigErrCh := make(chan error, 1)
	r.setupSignalHandleFn = func(_ context.Context, _ *config.Config, _ *string, _ *zap.Logger) (chan os.Signal, chan error) {
		return nil, sigErrCh
	}
	r.prepareSnapshotFn = func(context.Context, *config.Config, string, *zap.Logger) (string, chan error, func(), error) {
		return "snap", nil, func() {}, nil
	}
	r.executeClientFn = func(context.Context, func(context.Context, string, string) error, string, string, chan error, chan error, *zap.Logger) error {
		return <-sigErrCh
	}

	err := r.Run(cfg, []string{"vol"}, logger)
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

			cfg := &config.Config{StdoutMode: true, GRPCConnect: tc.grpcConnect, GRPCSetupTimeout: time.Second, SourceType: "file"}
			logger := zap.NewNop()
			r := NewRunner()
			r.startGRPCServerFn = func(context.Context, *config.Config, *zap.Logger) (func(), <-chan error, error) {
				ch := make(chan error)
				close(ch)
				return func() {}, ch, nil
			}

			if tc.hbChannel {
				hbErrCh := make(chan error)
				r.clientHandshakeFn = func(context.Context, *config.Config, *zap.Logger) (func(), chan error, error) {
					return func() { close(hbErrCh) }, hbErrCh, nil
				}
			} else {
				r.clientHandshakeFn = func(context.Context, *config.Config, *zap.Logger) (func(), chan error, error) {
					return func() {}, nil, nil
				}
			}

			r.setupSignalHandleFn = func(context.Context, *config.Config, *string, *zap.Logger) (chan os.Signal, chan error) {
				return nil, make(chan error, 1)
			}

			r.selectTransportFn = func(*config.Config, *zap.Logger) (transport.Interface, error) { return nil, nil }

			r.prepareSnapshotFn = func(context.Context, *config.Config, string, *zap.Logger) (string, chan error, func(), error) {
				return "snap", nil, func() {}, nil
			}

			r.executeClientFn = func(context.Context, func(context.Context, string, string) error, string, string, chan error, chan error, *zap.Logger) error {
				return nil
			}

			if err := r.Run(cfg, []string{"vol"}, logger); err != nil {
				t.Fatalf("Run returned error: %v", err)
			}

			time.Sleep(10 * time.Millisecond)
		})
	}
}
