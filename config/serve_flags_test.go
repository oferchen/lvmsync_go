package config

import (
	"testing"
	"time"
)

func TestServeFlagParsing(t *testing.T) {
	resetFlags([]string{"--serve", "--serve_listen", "localhost:9900", "--serve_protocol", "p", "--serve_algorithm", "a", "--serve_test_space", "t", "--serve_policy", "accept", "--serve_accept_timeout", "2s"})
	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	fs := NewFlagSets(defaults)
	cfg, err := LoadConfig(fs, defaults)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if !cfg.Serve {
		t.Fatalf("expected Serve true")
	}
	if cfg.ServeListen != "localhost:9900" || cfg.ServeProtocol != "p" || cfg.ServeAlgorithm != "a" || cfg.ServeTestSpace != "t" || cfg.ServePolicy != "accept" || cfg.ServeAcceptTimeout != 2*time.Second {
		t.Fatalf("unexpected serve config: %+v", cfg)
	}
}
