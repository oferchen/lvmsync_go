package quic

import (
	"crypto/tls"
	"testing"
)

func TestNewTLSConfigInsecure(t *testing.T) {
	cfg, err := NewTLSConfig(Config{AllowInsecure: true}, true)
	if err != nil {
		t.Fatalf("NewTLSConfig: %v", err)
	}
	if cfg.MinVersion != tls.VersionTLS13 || cfg.MaxVersion != tls.VersionTLS13 {
		t.Fatalf("unexpected tls versions")
	}
	if !cfg.InsecureSkipVerify {
		t.Fatalf("expected InsecureSkipVerify")
	}
}

func TestNewTLSConfigMissingCert(t *testing.T) {
	if _, err := NewTLSConfig(Config{}, true); err == nil {
		t.Fatalf("expected error")
	}
}

func TestNewQUICConfig(t *testing.T) {
	qc := NewQUICConfig()
	if qc.Allow0RTT {
		t.Fatalf("0-RTT should be disabled")
	}
}
