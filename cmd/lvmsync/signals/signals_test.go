package signals

import (
	"os"
	"strings"
	"testing"

	"go.uber.org/zap"

	"lvmsync_go/config"
)

func TestHandle(t *testing.T) {
	cfg := &config.Config{SkipSnapshotCreation: true}
	sigCh := make(chan os.Signal, 1)
	errCh := make(chan error, 1)
	var snap string
	logger := zap.NewNop()
	go Handle(cfg, logger, sigCh, &snap, errCh)
	sigCh <- os.Interrupt
	err := <-errCh
	if err == nil || !strings.Contains(err.Error(), "received signal") {
		t.Fatalf("expected signal error, got %v", err)
	}
}
