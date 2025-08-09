package signals

import (
	"os"
	"strings"
	"testing"

	"lvmsync_go/config"
)

func TestHandle(t *testing.T) {
	cfg := &config.Config{SkipSnapshotCreation: true}
	sigCh := make(chan os.Signal, 1)
	errCh := make(chan error, 1)
	var snap string
	go Handle(cfg, sigCh, &snap, errCh)
	sigCh <- os.Interrupt
	err := <-errCh
	if err == nil || !strings.Contains(err.Error(), "received signal") {
		t.Fatalf("expected signal error, got %v", err)
	}
}
