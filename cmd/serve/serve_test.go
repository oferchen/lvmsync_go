package serve

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"testing"
	"time"

	quic "github.com/quic-go/quic-go"
	"go.uber.org/zap"

	"lvmsync_go/config"
)

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer l.Close()
	return l.LocalAddr().(*net.UDPAddr).Port
}

func TestRunSuccess(t *testing.T) {
	port := freePort(t)
	cfg := &config.Config{
		Serve:              true,
		ServeListen:        fmt.Sprintf("127.0.0.1:%d", port),
		ServeProtocol:      "lvmsync",
		ServeAlgorithm:     "sha256",
		ServeTestSpace:     "ts",
		ServePolicy:        "accept",
		ServeAcceptTimeout: time.Second,
	}
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error)
	go func() { errCh <- Run(ctx, cfg, zap.NewNop()) }()
	time.Sleep(100 * time.Millisecond)

	tlsConf := &tls.Config{InsecureSkipVerify: true, NextProtos: []string{cfg.ServeProtocol}}
	conn, err := quic.DialAddr(ctx, cfg.ServeListen, tlsConf, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	stream, err := conn.OpenStreamSync(ctx)
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	fmt.Fprintf(stream, "%s|%s\n", cfg.ServeAlgorithm, cfg.ServeTestSpace)
	time.Sleep(50 * time.Millisecond)
	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
}

func TestRunHandshakeMismatch(t *testing.T) {
	port := freePort(t)
	cfg := &config.Config{
		Serve:              true,
		ServeListen:        fmt.Sprintf("127.0.0.1:%d", port),
		ServeProtocol:      "lvmsync",
		ServeAlgorithm:     "sha256",
		ServeTestSpace:     "ts",
		ServePolicy:        "accept",
		ServeAcceptTimeout: time.Second,
	}
	errCh := make(chan error)
	go func() { errCh <- Run(context.Background(), cfg, zap.NewNop()) }()
	time.Sleep(100 * time.Millisecond)

	tlsConf := &tls.Config{InsecureSkipVerify: true, NextProtos: []string{cfg.ServeProtocol}}
	conn, err := quic.DialAddr(context.Background(), cfg.ServeListen, tlsConf, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	stream, err := conn.OpenStreamSync(context.Background())
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	fmt.Fprintf(stream, "wrong|%s\n", cfg.ServeTestSpace)
	if err := <-errCh; err == nil {
		t.Fatalf("expected error from Run")
	}
}
