package serve

import (
	"bufio"
	"context"
	"io"
	"net"
	"testing"

	"go.uber.org/zap"

	"lvmsync_go/config"
	qn "lvmsync_go/quic"
)

func TestServeFlagBindingSuccess(t *testing.T) {
	resetFlags([]string{"--serve", "--serve_listen", "localhost:9900", "--serve_protocol", "p", "--serve_algorithm", "a", "--serve_test_space", "t", "--serve_policy", "accept"})
	defaults, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	fs := config.NewFlagSets(defaults)
	cfg, err := config.LoadConfig(fs, defaults)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if !cfg.Serve {
		t.Fatalf("expected Serve true")
	}
	if cfg.ServeListen != "localhost:9900" || cfg.ServeProtocol != "p" || cfg.ServeAlgorithm != "a" || cfg.ServeTestSpace != "t" || cfg.ServePolicy != "accept" {
		t.Fatalf("unexpected serve config: %+v", cfg)
	}

	server, client := net.Pipe()
	orig := acceptFunc
	acceptFunc = func(ctx context.Context, cfg *config.Config) (io.ReadWriteCloser, error) { return server, nil }
	defer func() { acceptFunc = orig }()

	errCh := make(chan error, 1)
	go func() { errCh <- Run(context.Background(), cfg, zap.NewNop()) }()
	hs := qn.Negotiation{Protocol: "p", Algorithm: "a", TestSpace: "t"}
	if err := qn.WriteNegotiation(client, hs); err != nil {
		t.Fatalf("write negotiation: %v", err)
	}
	if got, err := qn.ReadNegotiation(bufio.NewReader(client)); err != nil || got != hs {
		t.Fatalf("unexpected ack: %v %v", got, err)
	}
	client.Close()
	if err := <-errCh; err != nil {
		t.Fatalf("Run error: %v", err)
	}
}

func TestServeFlagBindingInvalidPolicy(t *testing.T) {
	resetFlags([]string{"--serve", "--serve_listen", "localhost:9900", "--serve_protocol", "p", "--serve_algorithm", "a", "--serve_test_space", "t", "--serve_policy", "deny"})
	defaults, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	fs := config.NewFlagSets(defaults)
	cfg, err := config.LoadConfig(fs, defaults)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.ServePolicy != "deny" {
		t.Fatalf("expected deny policy, got %q", cfg.ServePolicy)
	}

	server, client := net.Pipe()
	orig := acceptFunc
	acceptFunc = func(ctx context.Context, cfg *config.Config) (io.ReadWriteCloser, error) { return server, nil }
	defer func() { acceptFunc = orig }()

	errCh := make(chan error, 1)
	go func() { errCh <- Run(context.Background(), cfg, zap.NewNop()) }()
	hs := qn.Negotiation{Protocol: "p", Algorithm: "a", TestSpace: "t"}
	if err := qn.WriteNegotiation(client, hs); err != nil {
		t.Fatalf("write negotiation: %v", err)
	}
	if _, err := qn.ReadNegotiation(bufio.NewReader(client)); err != nil {
		t.Fatalf("read negotiation: %v", err)
	}
	client.Close()
	if err := <-errCh; err == nil {
		t.Fatalf("expected Run error")
	}
}
