package serve

import (
	"bufio"
	"io"
	"net"
	"testing"

	"go.uber.org/zap"

	"lvmsync_go/config"
	qn "lvmsync_go/quic"
)

func TestRunAcceptsTransfer(t *testing.T) {
	server, client := net.Pipe()
	orig := acceptFunc
	acceptFunc = func(cfg *config.Config) (io.ReadWriteCloser, error) { return server, nil }
	defer func() { acceptFunc = orig }()

	cfg := &config.Config{ServeProtocol: "proto", ServeAlgorithm: "alg", ServeTestSpace: "ts", ServePolicy: "accept"}
	errCh := make(chan error, 1)
	go func() { errCh <- Run(cfg, zap.NewNop()) }()

	hs := qn.Negotiation{Protocol: "proto", Algorithm: "alg", TestSpace: "ts"}
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
