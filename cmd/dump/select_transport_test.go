package dump

import (
	"strings"
	"testing"

	"lvmsync_go/config"
	_ "lvmsync_go/transport/ssh"

	"go.uber.org/zap"
)

func TestSelectTransportNoConfig(t *testing.T) {
	cfg := &config.Config{}
	logger := zap.NewNop()
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
	logger := zap.NewNop()
	_, err := SelectTransport(cfg, logger)
	if err == nil || !strings.Contains(err.Error(), "no supported") {
		t.Fatalf("expected transport error, got %v", err)
	}
}

func TestSelectTransportOrder(t *testing.T) {
	cfg := &config.Config{Transport: "bogus,ssh"}
	logger := zap.NewNop()
	tr, err := SelectTransport(cfg, logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tr == nil || tr.Name() != "ssh" {
		t.Fatalf("expected ssh transport, got %v", tr)
	}
}
