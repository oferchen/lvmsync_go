package dump

import (
	"strings"
	"testing"

	"lvmsync_go/config"

	"go.uber.org/zap"
)

func TestSelectTransportNoConfig(t *testing.T) {
	cfg := &config.Config{}
	logger := zap.NewNop()
	if err := SelectTransport(cfg, logger); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSelectTransportError(t *testing.T) {
	cfg := &config.Config{Transport: "tcp+tls"}
	logger := zap.NewNop()
	err := SelectTransport(cfg, logger)
	if err == nil || !strings.Contains(err.Error(), "not implemented") {
		t.Fatalf("expected transport error, got %v", err)
	}
}
