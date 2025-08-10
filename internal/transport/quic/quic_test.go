package quic

import (
	"testing"

	"go.uber.org/zap"

	"lvmsync_go/config"
	"lvmsync_go/internal/transport"
)

func TestQUICRegistered(t *testing.T) {
	if _, ok := transport.Get("quic"); !ok {
		t.Fatalf("quic transport not registered")
	}
}

func TestQUICNew(t *testing.T) {
	if _, _, err := New(&config.Config{}, zap.NewNop()); err != nil {
		t.Fatalf("New: %v", err)
	}
}
