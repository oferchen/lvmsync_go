package transport_test

import (
	"strings"
	"testing"

	dump "lvmsync_go/cmd/dump"
	"lvmsync_go/internal/config"
	_ "lvmsync_go/transport/ssh"

	"go.uber.org/zap/zaptest"
)

func TestDefaultOrderAndForcedTransport(t *testing.T) {
	defaults, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("defaults: %v", err)
	}
	got := strings.Split(defaults.Transport, ",")
	want := []string{"ssh", "tcp+tls", "h2", "quic"}
	if len(got) != len(want) {
		t.Fatalf("default order %v want %v", got, want)
	}
	for i, v := range want {
		if got[i] != v {
			t.Fatalf("default order %v want %v", got, want)
		}
	}

	logger := zaptest.NewLogger(t)
	defer logger.Sync()
	cfg := &config.Config{Transport: "ssh", SSHUser: "user", SSHPassword: "pass", AllowInsecure: true}
	tr, err := dump.SelectTransport(cfg, logger)
	if err != nil {
		t.Fatalf("select transport: %v", err)
	}
	if tr == nil || tr.Name() != "ssh" {
		t.Fatalf("expected ssh, got %v", tr)
	}

	cfg.Transport = "bogus"
	if _, err := dump.SelectTransport(cfg, logger); err == nil || !strings.Contains(err.Error(), "unsupported transport") {
		t.Fatalf("expected unsupported transport error, got %v", err)
	}
}
