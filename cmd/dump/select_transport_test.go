//go:build rsync

package dump

import (
	"context"
	"errors"
	"net"
	"reflect"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
	"go.uber.org/zap/zaptest/observer"

	"lvmsync_go/common"
	"lvmsync_go/internal/config"
	"lvmsync_go/transport"
	_ "lvmsync_go/transport/rsyncwire"
	_ "lvmsync_go/transport/ssh"
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
	if err == nil || !strings.Contains(err.Error(), "unsupported transport") {
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

func TestSelectTransportRsyncRequiresAllowInsecure(t *testing.T) {
	cfg := &config.Config{Transport: "rsync"}
	logger := zaptest.NewLogger(t)
	defer logger.Sync()
	if _, err := SelectTransport(cfg, logger); err == nil || !strings.Contains(err.Error(), "unsupported transport") {
		t.Fatalf("expected unsupported transport error, got %v", err)
	}
}

func TestSelectTransportRsyncAllowInsecure(t *testing.T) {
	cfg := &config.Config{Transport: "rsync", AllowInsecure: true}
	logger := zaptest.NewLogger(t)
	defer logger.Sync()
	tr, err := SelectTransport(cfg, logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tr == nil || tr.Name() != "rsync" {
		t.Fatalf("expected rsync transport, got %v", tr)
	}
}

type stubTransport struct {
	name    string
	dialErr error
}

func (s *stubTransport) Name() string { return s.name }

func (s *stubTransport) Dial(ctx context.Context, address string) (net.Conn, error) {
	if s.dialErr != nil {
		return nil, s.dialErr
	}
	c1, c2 := net.Pipe()
	go c2.Close()
	return c1, nil
}

func (s *stubTransport) Listen(ctx context.Context, address string) (net.Listener, error) {
	return nil, errors.New("not implemented")
}

func (s *stubTransport) Negotiate(ctx context.Context, conn net.Conn, role transport.Role, hs common.Handshake) (common.Handshake, error) {
	return common.Handshake{}, errors.New("not implemented")
}

func TestDialWithFallbackLogsAttempts(t *testing.T) {
	failErr := errors.New("dial failed")
	if err := transport.Register("failssh", func(cfg transport.Config) (transport.Interface, error) {
		return &stubTransport{name: "failssh", dialErr: failErr}, nil
	}); err != nil {
		t.Fatalf("register failssh: %v", err)
	}
	if err := transport.Register("oktcp", func(cfg transport.Config) (transport.Interface, error) { return &stubTransport{name: "oktcp"}, nil }); err != nil {
		t.Fatalf("register oktcp: %v", err)
	}
	core, obs := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	tr, conn, err := transport.DialWithFallback(ctx, "127.0.0.1:0", []string{"failssh", "oktcp"}, transport.Config{Logger: logger})
	if err != nil {
		t.Fatalf("DialWithFallback: %v", err)
	}
	conn.Close()
	if tr.Name() != "oktcp" {
		t.Fatalf("expected oktcp, got %s", tr.Name())
	}
	var seq []string
	for _, e := range obs.All() {
		if e.Message == "dial_attempt" {
			if n, ok := e.ContextMap()["transport"].(string); ok {
				seq = append(seq, n)
			}
		}
	}
	expected := []string{"failssh", "oktcp"}
	if !reflect.DeepEqual(seq, expected) {
		t.Fatalf("attempt sequence %v, expected %v", seq, expected)
	}
}
