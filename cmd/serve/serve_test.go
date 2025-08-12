package serve

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"testing"

	q "github.com/quic-go/quic-go"
	"github.com/spf13/pflag"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"lvmsync_go/config"
	qn "lvmsync_go/quic"
)

type nopRWCloser struct{ io.ReadCloser }

func (nopRWCloser) Write(p []byte) (int, error) { return len(p), nil }

func TestRunAcceptsTransfer(t *testing.T) {
	server, client := net.Pipe()
	orig := acceptFunc
	acceptFunc = func(ctx context.Context, cfg *config.Config) (io.ReadWriteCloser, error) { return server, nil }
	defer func() { acceptFunc = orig }()

	cfg := &config.Config{ServeProtocol: "proto", ServeAlgorithm: "alg", ServeTestSpace: "ts", ServePolicy: "accept"}
	errCh := make(chan error, 1)
	go func() { errCh <- Run(context.Background(), cfg, zap.NewNop()) }()

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

// resetFlags replaces the global command line flags and sets os.Args.
func resetFlags(args []string) {
	pflag.CommandLine = pflag.NewFlagSet(os.Args[0], pflag.ContinueOnError)
	pflag.CommandLine.SetOutput(io.Discard)
	os.Args = append([]string{"test"}, args...)
}

func TestServeFlagParsing(t *testing.T) {
	resetFlags([]string{"--serve", "--serve_listen", "localhost:9900", "--serve_protocol", "p", "--serve_algorithm", "a", "--serve_test_space", "t", "--serve_policy", "accept", "--allow_insecure"})
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
}

func TestRunNegotiationMismatch(t *testing.T) {
	server, client := net.Pipe()
	orig := acceptFunc
	acceptFunc = func(ctx context.Context, cfg *config.Config) (io.ReadWriteCloser, error) { return server, nil }
	defer func() { acceptFunc = orig }()

	cfg := &config.Config{ServeProtocol: "proto", ServeAlgorithm: "alg", ServeTestSpace: "ts", ServePolicy: "accept"}
	errCh := make(chan error, 1)
	go func() { errCh <- Run(context.Background(), cfg, zap.NewNop()) }()

	if err := qn.WriteNegotiation(client, qn.Negotiation{Protocol: "proto", Algorithm: "other", TestSpace: "ts"}); err != nil {
		t.Fatalf("write negotiation: %v", err)
	}
	client.Close()
	if err := <-errCh; err == nil {
		t.Fatalf("expected error")
	}
}

func TestRunRejectsPolicy(t *testing.T) {
	server, client := net.Pipe()
	orig := acceptFunc
	acceptFunc = func(ctx context.Context, cfg *config.Config) (io.ReadWriteCloser, error) { return server, nil }
	defer func() { acceptFunc = orig }()

	cfg := &config.Config{ServeProtocol: "proto", ServeAlgorithm: "alg", ServeTestSpace: "ts", ServePolicy: "deny"}
	errCh := make(chan error, 1)
	go func() { errCh <- Run(context.Background(), cfg, zap.NewNop()) }()

	hs := qn.Negotiation{Protocol: "proto", Algorithm: "alg", TestSpace: "ts"}
	if err := qn.WriteNegotiation(client, hs); err != nil {
		t.Fatalf("write negotiation: %v", err)
	}
	if _, err := qn.ReadNegotiation(bufio.NewReader(client)); err != nil {
		t.Fatalf("read negotiation: %v", err)
	}
	client.Close()
	if err := <-errCh; err == nil {
		t.Fatalf("expected error")
	}
}

type errCloseConn struct{ net.Conn }

func (e errCloseConn) Close() error { return fmt.Errorf("close fail") }

func TestRunLogsStreamCloseError(t *testing.T) {
	server, client := net.Pipe()
	orig := acceptFunc
	acceptFunc = func(ctx context.Context, cfg *config.Config) (io.ReadWriteCloser, error) {
		return errCloseConn{server}, nil
	}
	defer func() { acceptFunc = orig }()

	cfg := &config.Config{ServeProtocol: "proto", ServeAlgorithm: "alg", ServeTestSpace: "ts", ServePolicy: "accept"}
	core, logs := observer.New(zap.WarnLevel)
	errCh := make(chan error, 1)
	go func() { errCh <- Run(context.Background(), cfg, zap.New(core)) }()

	hs := qn.Negotiation{Protocol: "proto", Algorithm: "alg", TestSpace: "ts"}
	if err := qn.WriteNegotiation(client, hs); err != nil {
		t.Fatalf("write negotiation: %v", err)
	}
	if _, err := qn.ReadNegotiation(bufio.NewReader(client)); err != nil {
		t.Fatalf("read negotiation: %v", err)
	}
	client.Close()

	if err := <-errCh; err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if logs.FilterMessage("serve: stream close").Len() != 1 {
		t.Fatalf("expected stream close warning log")
	}
}

type fakeConn struct{ closed bool }

func (f *fakeConn) CloseWithError(code q.ApplicationErrorCode, msg string) error {
	f.closed = true
	return nil
}

type fakeListener struct{ closed bool }

func (f *fakeListener) Close() error {
	f.closed = true
	return nil
}

func TestQuicStreamCloseClosesResources(t *testing.T) {
	qs := &quicStream{
		ReadWriteCloser: nopRWCloser{io.NopCloser(strings.NewReader(""))},
		conn:            &fakeConn{},
		listener:        &fakeListener{},
	}
	if err := qs.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !qs.conn.(*fakeConn).closed || !qs.listener.(*fakeListener).closed {
		t.Fatalf("expected conn and listener closed")
	}
}
