package root

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/oferchen/lvmsync_go/internal/config"
	"github.com/oferchen/lvmsync_go/transport"
)

func TestPrepareClientSelectTransportError(t *testing.T) {
	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig error: %v", err)
	}
	r := NewRunnerWithDeps(&Runner{
		selectTransportFn: func(*config.Config, *zap.Logger) (transport.Interface, error) {
			return nil, errors.New("boom")
		},
	})
	if _, _, _, _, _, _, err := r.prepareClient(cfg, []string{"/src", "/dst"}, zap.NewNop()); err == nil || !strings.Contains(err.Error(), "select transport") {
		t.Fatalf("expected select transport error, got: %v", err)
	}
}

func TestPrepareClientMissingArguments(t *testing.T) {
	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig error: %v", err)
	}
	r := NewRunnerWithDeps(&Runner{
		selectTransportFn: func(*config.Config, *zap.Logger) (transport.Interface, error) { return nil, nil },
		setupSignalHandleFn: func(context.Context, *config.Config, *string, *zap.Logger) (chan os.Signal, chan error) {
			return make(chan os.Signal), make(chan error)
		},
	})
	if _, _, _, _, _, _, err := r.prepareClient(cfg, []string{"/src"}, zap.NewNop()); err == nil || err.Error() != "invalid arguments" {
		t.Fatalf("expected invalid arguments error, got: %v", err)
	}
}

func TestPrepareClientDirectDeviceRequiresForceOffline(t *testing.T) {
	devPath := createBlockDevice(t)
	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig error: %v", err)
	}
	r := NewRunnerWithDeps(&Runner{
		selectTransportFn: func(*config.Config, *zap.Logger) (transport.Interface, error) { return nil, nil },
		setupSignalHandleFn: func(context.Context, *config.Config, *string, *zap.Logger) (chan os.Signal, chan error) {
			return make(chan os.Signal), make(chan error)
		},
	})
	if _, _, _, _, _, _, err := r.prepareClient(cfg, []string{"/src", devPath}, zap.NewNop()); err == nil || err.Error() != "direct device writes require --force-offline" {
		t.Fatalf("expected force-offline error, got: %v", err)
	}
}
