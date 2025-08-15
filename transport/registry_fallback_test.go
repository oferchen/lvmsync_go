package transport

import (
	"context"
	"errors"
	"fmt"
	"net"
	"reflect"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"lvmsync_go/common"
)

// stubTransport implements Interface with configurable dial behavior.
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
	return nil, fmt.Errorf("not implemented")
}

func (s *stubTransport) Negotiate(ctx context.Context, conn net.Conn, role Role, hs common.Handshake) (common.Handshake, error) {
	return common.Handshake{}, fmt.Errorf("not implemented")
}

// dialWithFallback tries transports in order until one successfully dials.
func dialWithFallback(ctx context.Context, address string, names []string, logger *zap.Logger) (string, error) {
	for _, name := range names {
		logger.Info("dial_attempt", zap.String("transport", name))
		tr, err := Get(name, Config{Logger: logger})
		if err != nil {
			logger.Warn("get_failed", zap.String("transport", name), zap.Error(err))
			continue
		}
		conn, err := tr.Dial(ctx, address)
		if err != nil {
			logger.Warn("dial_failed", zap.String("transport", name), zap.Error(err))
			continue
		}
		conn.Close()
		logger.Info("dial_success", zap.String("transport", name))
		return name, nil
	}
	return "", fmt.Errorf("all transports failed")
}

func TestRegistryDialFallbackSequence(t *testing.T) {
	regMu.Lock()
	original := registry
	registry = map[string]Factory{}
	regMu.Unlock()
	defer func() {
		regMu.Lock()
		registry = original
		regMu.Unlock()
	}()

	failErr := errors.New("dial error")
	Register("quic", func(Config) (Interface, error) { return &stubTransport{name: "quic", dialErr: failErr}, nil })
	Register("h2", func(Config) (Interface, error) { return &stubTransport{name: "h2", dialErr: failErr}, nil })
	Register("tcp+tls", func(Config) (Interface, error) { return &stubTransport{name: "tcp+tls", dialErr: failErr}, nil })
	Register("ssh", func(Config) (Interface, error) { return &stubTransport{name: "ssh"}, nil })

	core, obs := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	defer logger.Sync()

	name, err := dialWithFallback(context.Background(), "127.0.0.1:0", []string{"quic", "h2", "tcp+tls", "ssh"}, logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "ssh" {
		t.Fatalf("expected ssh, got %s", name)
	}

	var seq []string
	for _, entry := range obs.All() {
		if entry.Message == "dial_attempt" {
			if n, ok := entry.ContextMap()["transport"].(string); ok {
				seq = append(seq, n)
			}
		}
	}
	expected := []string{"quic", "h2", "tcp+tls", "ssh"}
	if !reflect.DeepEqual(seq, expected) {
		t.Fatalf("attempt sequence %v, expected %v", seq, expected)
	}
}
