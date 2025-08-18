package root

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"testing"

	"go.uber.org/zap"

	"lvmsync_go/internal/config"
	"lvmsync_go/internal/privilege"
	"lvmsync_go/transport"
)

type stubEscalator struct{ err error }

func (s stubEscalator) Ensure(context.Context) error { return s.err }

func (s stubEscalator) Command(context.Context, string, ...string) *exec.Cmd {
	return exec.Command("true")
}

var _ privilege.Escalator = (*stubEscalator)(nil)

func TestPrepareClientCreatesSnapshotAndCleanup(t *testing.T) {
	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig error: %v", err)
	}

	snapCleanupCalled := false
	var snapshotPtr *string
	r := NewRunnerWithDeps(&Runner{
		selectTransportFn: func(*config.Config, *zap.Logger) (transport.Interface, error) { return nil, nil },
		setupSignalHandleFn: func(ctx context.Context, cfg *config.Config, snap *string, _ *zap.Logger) (chan os.Signal, chan error) {
			snapshotPtr = snap
			return make(chan os.Signal), make(chan error)
		},
		prepareSnapshotFn: func(context.Context, *config.Config, string, *zap.Logger) (string, chan error, func(), error) {
			return "snap-path", make(chan error), func() { snapCleanupCalled = true }, nil
		},
	})

	logger := zap.NewNop()
	ctx, cleanup, snapPath, destPath, sigCh, monitorCh, err := r.prepareClient(cfg, []string{"/dev/vg/orig", "/dest"}, logger)
	if err != nil {
		t.Fatalf("prepareClient error: %v", err)
	}
	if ctx == nil || cleanup == nil || sigCh == nil || monitorCh == nil {
		t.Fatalf("expected non-nil results")
	}
	if snapPath != "snap-path" {
		t.Fatalf("unexpected snapshot path: %s", snapPath)
	}
	if destPath != "/dest" {
		t.Fatalf("unexpected dest path: %s", destPath)
	}
	if snapshotPtr == nil || *snapshotPtr != snapPath {
		t.Fatalf("snapshot path pointer not updated")
	}

	cleanup()
	if !snapCleanupCalled {
		t.Fatalf("snapshot cleanup not invoked")
	}
}

func TestPrepareClientSnapshotError(t *testing.T) {
	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig error: %v", err)
	}

	r := NewRunnerWithDeps(&Runner{
		selectTransportFn: func(*config.Config, *zap.Logger) (transport.Interface, error) { return nil, nil },
		setupSignalHandleFn: func(ctx context.Context, cfg *config.Config, snap *string, _ *zap.Logger) (chan os.Signal, chan error) {
			return make(chan os.Signal), make(chan error)
		},
		prepareSnapshotFn: func(context.Context, *config.Config, string, *zap.Logger) (string, chan error, func(), error) {
			return "", nil, nil, errors.New("snap error")
		},
	})

	logger := zap.NewNop()
	if _, cleanup, _, _, _, _, err := r.prepareClient(cfg, []string{"/dev/vg/orig", "/dest"}, logger); err == nil {
		t.Fatalf("expected snapshot error")
	} else if cleanup != nil {
		t.Fatalf("expected nil cleanup on error")
	}
}

func TestDispatchSubcommand(t *testing.T) {
	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig error: %v", err)
	}
	logger := zap.NewNop()

	manifestCalled := false
	verifyCalled := false
	r := NewRunnerWithDeps(&Runner{
		runManifestFn: func(c *config.Config, args []string, _ *zap.Logger) error {
			manifestCalled = true
			if args[0] != "arg1" || args[1] != "arg2" {
				t.Fatalf("unexpected manifest args: %v", args)
			}
			return nil
		},
		runVerifyFn: func(args []string, _ *zap.Logger) error {
			verifyCalled = true
			if args[0] != "varg" {
				t.Fatalf("unexpected verify args: %v", args)
			}
			return nil
		},
	})

	handled, err := r.dispatchSubcommand(cfg, []string{"manifest", "arg1", "arg2"}, logger)
	if err != nil || !handled || !manifestCalled {
		t.Fatalf("manifest not handled: handled=%v err=%v called=%v", handled, err, manifestCalled)
	}

	handled, err = r.dispatchSubcommand(cfg, []string{"verify", "varg"}, logger)
	if err != nil || !handled || !verifyCalled {
		t.Fatalf("verify not handled: handled=%v err=%v called=%v", handled, err, verifyCalled)
	}

	if handled, err = r.dispatchSubcommand(cfg, []string{"other"}, logger); handled || err != nil {
		t.Fatalf("unexpected handling for unknown subcommand: handled=%v err=%v", handled, err)
	}
}

func TestDispatchSubcommandError(t *testing.T) {
	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig error: %v", err)
	}
	r := NewRunnerWithDeps(&Runner{
		runManifestFn: func(*config.Config, []string, *zap.Logger) error { return errors.New("boom") },
	})
	handled, err := r.dispatchSubcommand(cfg, []string{"manifest"}, zap.NewNop())
	if !handled || err == nil {
		t.Fatalf("expected manifest error")
	}
}

func TestRunSubcommandHandled(t *testing.T) {
	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig error: %v", err)
	}
	called := false
	r := NewRunnerWithDeps(&Runner{
		runManifestFn: func(*config.Config, []string, *zap.Logger) error { called = true; return nil },
		prepareSnapshotFn: func(context.Context, *config.Config, string, *zap.Logger) (string, chan error, func(), error) {
			t.Fatalf("prepareSnapshot should not be called")
			return "", nil, nil, nil
		},
	})
	if err := r.Run(cfg, []string{"manifest"}, zap.NewNop()); err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if !called {
		t.Fatalf("manifest handler not invoked")
	}
}

func TestRunSuccess(t *testing.T) {
	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig error: %v", err)
	}
	r := NewRunnerWithDeps(&Runner{
		selectTransportFn: func(*config.Config, *zap.Logger) (transport.Interface, error) { return nil, nil },
		setupSignalHandleFn: func(ctx context.Context, cfg *config.Config, snap *string, _ *zap.Logger) (chan os.Signal, chan error) {
			return make(chan os.Signal), make(chan error)
		},
		prepareSnapshotFn: func(context.Context, *config.Config, string, *zap.Logger) (string, chan error, func(), error) {
			return "snap", make(chan error), func() {}, nil
		},
		executeClientFn: func(context.Context, func(context.Context, string, string) error, string, string, chan error, chan error, *zap.Logger) error {
			return nil
		},
	})
	if err := r.Run(cfg, []string{"/src", "/dst"}, zap.NewNop()); err != nil {
		t.Fatalf("Run error: %v", err)
	}
}

func TestRunPrepareSnapshotError(t *testing.T) {
	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig error: %v", err)
	}
	r := NewRunnerWithDeps(&Runner{
		selectTransportFn: func(*config.Config, *zap.Logger) (transport.Interface, error) { return nil, nil },
		setupSignalHandleFn: func(ctx context.Context, cfg *config.Config, snap *string, _ *zap.Logger) (chan os.Signal, chan error) {
			return make(chan os.Signal), make(chan error)
		},
		prepareSnapshotFn: func(context.Context, *config.Config, string, *zap.Logger) (string, chan error, func(), error) {
			return "", nil, nil, errors.New("snap fail")
		},
	})
	if err := r.Run(cfg, []string{"/src", "/dst"}, zap.NewNop()); err == nil {
		t.Fatalf("expected snapshot error")
	}
}

func TestRunExecuteClientError(t *testing.T) {
	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig error: %v", err)
	}
	r := NewRunnerWithDeps(&Runner{
		selectTransportFn: func(*config.Config, *zap.Logger) (transport.Interface, error) { return nil, nil },
		setupSignalHandleFn: func(ctx context.Context, cfg *config.Config, snap *string, _ *zap.Logger) (chan os.Signal, chan error) {
			return make(chan os.Signal), make(chan error)
		},
		prepareSnapshotFn: func(context.Context, *config.Config, string, *zap.Logger) (string, chan error, func(), error) {
			return "snap", make(chan error), func() {}, nil
		},
		executeClientFn: func(context.Context, func(context.Context, string, string) error, string, string, chan error, chan error, *zap.Logger) error {
			return errors.New("exec fail")
		},
	})
	if err := r.Run(cfg, []string{"/src", "/dst"}, zap.NewNop()); err == nil {
		t.Fatalf("expected execute error")
	}
}

func TestConfigure(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()

	t.Run("success", func(t *testing.T) {
		os.Args = []string{"cmd", "/src", "/dst"}
		cfg, args, logger, err := ConfigureWithEscalator(stubEscalator{})
		if err != nil {
			t.Fatalf("Configure error: %v", err)
		}
		if cfg == nil || logger == nil || len(args) != 2 {
			t.Fatalf("invalid configure results")
		}
	})

	t.Run("escalation failure", func(t *testing.T) {
		os.Args = []string{"cmd", "/src", "/dst"}
		_, _, _, err := ConfigureWithEscalator(stubEscalator{err: errors.New("boom")})
		if err == nil {
			t.Fatalf("expected configure error")
		}
	})
}

func TestConfigureConfigError(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()
	os.Args = []string{"cmd", "--compress-threshold", "2", "/src", "/dst"}
	if _, _, _, err := ConfigureWithEscalator(stubEscalator{}); err == nil {
		t.Fatalf("expected configure error")
	}
}

func TestExecuteRunError(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()
	os.Args = []string{"cmd", "/src", "/dst"}
	if err := Execute(); err == nil {
		t.Fatalf("expected execute error")
	}
}

func TestExecuteConfigureError(t *testing.T) {
	orig := os.Args
	defer func() { os.Args = orig }()
	os.Args = []string{"cmd", "--compress-threshold", "2", "/src", "/dst"}
	if err := Execute(); err == nil {
		t.Fatalf("expected configure error")
	}
}
