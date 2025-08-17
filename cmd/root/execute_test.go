package root

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"lvmsync_go/internal/config"
	privilege "lvmsync_go/internal/privilege"
	"lvmsync_go/transport"

	"go.uber.org/zap"
)

func TestExecuteSuccess(t *testing.T) {
	origArgs := os.Args
	os.Args = []string{origArgs[0], "/dev/src", "/tmp/dest"}
	defer func() { os.Args = origArgs }()

	origCaps := privilege.HasCaps
	privilege.HasCaps = func() bool { return true }
	defer func() { privilege.HasCaps = origCaps }()

	origRunner := defaultRunner
	defaultRunner = &Runner{
		selectTransportFn: func(*config.Config, *zap.Logger) (transport.Interface, error) { return nil, nil },
		startGRPCServerFn: func(context.Context, *config.Config, *zap.Logger) (func(), <-chan error, error) {
			ch := make(chan error)
			close(ch)
			return func() {}, ch, nil
		},
		clientHandshakeFn: func(context.Context, *config.Config, *zap.Logger) (func(), chan error, error) {
			return func() {}, nil, nil
		},
		setupSignalHandleFn: func(context.Context, *config.Config, *string, *zap.Logger) (chan os.Signal, chan error) {
			sigCh := make(chan os.Signal)
			errCh := make(chan error)
			go func() {
				time.Sleep(time.Millisecond)
				close(errCh)
			}()
			return sigCh, errCh
		},
		executeClientFn: func(context.Context, func(context.Context, string, string) error, string, string, chan error, chan error, *zap.Logger) error {
			return nil
		},
	}
	defer func() { defaultRunner = origRunner }()

	if err := Execute(); err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
}

func TestExecuteRunError(t *testing.T) {
	origArgs := os.Args
	os.Args = []string{origArgs[0], "/dev/src", "/tmp/dest"}
	defer func() { os.Args = origArgs }()

	origCaps := privilege.HasCaps
	privilege.HasCaps = func() bool { return true }
	defer func() { privilege.HasCaps = origCaps }()

	origRunner := defaultRunner
	defaultRunner = &Runner{
		selectTransportFn: func(*config.Config, *zap.Logger) (transport.Interface, error) { return nil, nil },
		startGRPCServerFn: func(context.Context, *config.Config, *zap.Logger) (func(), <-chan error, error) {
			ch := make(chan error)
			close(ch)
			return func() {}, ch, nil
		},
		clientHandshakeFn: func(context.Context, *config.Config, *zap.Logger) (func(), chan error, error) {
			return func() {}, nil, nil
		},
		setupSignalHandleFn: func(context.Context, *config.Config, *string, *zap.Logger) (chan os.Signal, chan error) {
			sigCh := make(chan os.Signal)
			errCh := make(chan error)
			go func() {
				time.Sleep(time.Millisecond)
				close(errCh)
			}()
			return sigCh, errCh
		},
		executeClientFn: func(context.Context, func(context.Context, string, string) error, string, string, chan error, chan error, *zap.Logger) error {
			return errors.New("boom")
		},
	}
	defer func() { defaultRunner = origRunner }()

	if err := Execute(); err == nil || err.Error() != "boom" {
		t.Fatalf("expected boom error, got %v", err)
	}
}
