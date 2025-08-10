package ssh

import (
	"testing"

	"go.uber.org/zap"

	"lvmsync_go/config"
	"lvmsync_go/internal/transport"
)

func TestSSHRegistered(t *testing.T) {
	if _, ok := transport.Get("ssh"); !ok {
		t.Fatalf("ssh transport not registered")
	}
}

func TestSSHNew(t *testing.T) {
	if _, _, err := New(&config.Config{}, zap.NewNop()); err != nil {
		t.Fatalf("New: %v", err)
	}
}
