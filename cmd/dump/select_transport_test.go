package dump

import (
	"strings"
	"testing"

	"lvmsync_go/internal/config"
	_ "lvmsync_go/transport/ssh"

	"go.uber.org/zap/zaptest"
)

func TestSelectTransportNoConfig(t *testing.T) {
	cfg := &config.Config{}
	logger := zaptest.NewLogger(t)
	defer logger.Sync()
	tr, err := SelectTransport(cfg, logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tr != nil {
		t.Fatalf("expected nil transport, got %v", tr)
	}
}

func TestSelectTransportError(t *testing.T) {
	cfg := &config.Config{Transport: "bogus"}
	logger := zaptest.NewLogger(t)
	defer logger.Sync()
	_, err := SelectTransport(cfg, logger)
	if err == nil || !strings.Contains(err.Error(), "no supported") {
		t.Fatalf("expected transport error, got %v", err)
	}
}

func TestSelectTransportOrder(t *testing.T) {
	cfg := &config.Config{Transport: "bogus,ssh", SSHUser: "test", SSHPassword: "pass", AllowInsecure: true}
	logger := zaptest.NewLogger(t)
	defer logger.Sync()
	tr, err := SelectTransport(cfg, logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tr == nil || tr.Name() != "ssh" {
		t.Fatalf("expected ssh transport, got %v", tr)
	}
}
