package root

import (
	"errors"
	"testing"

	"go.uber.org/zap"
	"lvmsync_go/config"
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

	startGRPCServer = func(_ *config.Config, _ *zap.Logger) (func(), error) {
		return func() { srvCleanCalled = true }, nil
	}
	clientHandshake = func(_ *config.Config, _ *zap.Logger) (func(), error) {
		return func() { clientCleanCalled = true }, nil
	}

	cleanupSrv, cleanupClient, err := SetupGRPC(cfg, logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cleanupSrv()
	cleanupClient()
	if !srvCleanCalled || !clientCleanCalled {
		t.Fatalf("cleanup functions not called")
	}
}

func TestSetupGRPCServerError(t *testing.T) {
	cfg := &config.Config{}
	logger := zap.NewNop()
	origStart := startGRPCServer
	defer func() { startGRPCServer = origStart }()
	startGRPCServer = func(_ *config.Config, _ *zap.Logger) (func(), error) {
		return nil, errors.New("srv fail")
	}
	if _, _, err := SetupGRPC(cfg, logger); err == nil {
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
	startGRPCServer = func(_ *config.Config, _ *zap.Logger) (func(), error) {
		return func() { srvCleanupCalled = true }, nil
	}
	clientHandshake = func(_ *config.Config, _ *zap.Logger) (func(), error) {
		return nil, errors.New("client fail")
	}
	if _, _, err := SetupGRPC(cfg, logger); err == nil {
		t.Fatalf("expected error")
	}
	if !srvCleanupCalled {
		t.Fatalf("server cleanup not called on error")
	}
}

func TestPrepareSnapshot(t *testing.T) {
	cfg := &config.Config{}
	logger := zap.NewNop()
	orig := prepareSnapshotFn
	defer func() { prepareSnapshotFn = orig }()
	prepareSnapshotFn = func(c *config.Config, v string, l *zap.Logger) (string, chan error, func(), error) {
		return "snap", nil, func() {}, nil
	}
	snap, ch, cleanup, err := PrepareSnapshot(cfg, "vol", logger)
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
	executeClientFn = func(f func(string, string) error, snap, dest string, sigErrCh, monitorErrCh chan error) error {
		called = true
		if snap != "s" || dest != "d" {
			t.Fatalf("unexpected args")
		}
		return f(snap, dest)
	}
	runDump = func(_ *config.Config, snap, dest string, _ *zap.Logger) error {
		if snap != "s" || dest != "d" {
			t.Fatalf("unexpected dump args")
		}
		return nil
	}
	if err := ExecuteClient(cfg, "s", "d", nil, nil, logger); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatalf("executeClientFn not called")
	}
}
