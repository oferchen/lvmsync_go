package main

import (
	"context"
	"net"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"

	"lvmsync_go/common"
	"lvmsync_go/transport"
)

// mockTransport implements transport.Interface for testing negotiation.
type mockTransport struct{ negotiated bool }

func (m *mockTransport) Name() string                                         { return "mock" }
func (m *mockTransport) Dial(context.Context, string) (net.Conn, error)       { return nil, nil }
func (m *mockTransport) Listen(context.Context, string) (net.Listener, error) { return nil, nil }
func (m *mockTransport) Negotiate(context.Context, net.Conn, transport.Role, common.Handshake) (common.Handshake, error) {
	m.negotiated = true
	return common.Handshake{}, nil
}

func TestFlagParsing(t *testing.T) {
	v := viper.New()
	cmd := &cobra.Command{}
	if err := bindFlags(cmd, v); err != nil {
		t.Fatalf("bindFlags: %v", err)
	}
	args := []string{"--module", "foo=/dev/null", "--module", "bar=/dev/zero", "--transport", "h2,tcp+tls", "--tcp-port", "9000"}
	if err := cmd.ParseFlags(args); err != nil {
		t.Fatalf("ParseFlags: %v", err)
	}
	opts := parseOptions(v)
	if len(opts.modules) != 2 || opts.modules["foo"] != "/dev/null" || opts.modules["bar"] != "/dev/zero" {
		t.Fatalf("modules parsed incorrectly: %+v", opts.modules)
	}
	if len(opts.transports) != 2 || opts.transports[0] != "h2" || opts.transports[1] != "tcp+tls" {
		t.Fatalf("transports parsed incorrectly: %+v", opts.transports)
	}
	if opts.tcpPort != 9000 {
		t.Fatalf("tcpPort parsed incorrectly: %d", opts.tcpPort)
	}
}

func TestModuleACL(t *testing.T) {
	tr := &mockTransport{}
	server, client := net.Pipe()
	done := make(chan error, 1)
	go func() {
		done <- handleConn(context.Background(), server, tr, map[string]string{"foo": "/dev/null"}, zap.NewNop())
	}()
	if _, err := client.Write([]byte("bar\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := <-done; err == nil {
		t.Fatalf("expected error for unauthorized module")
	}
}

func TestTransportNegotiation(t *testing.T) {
	tr := &mockTransport{}
	server, client := net.Pipe()
	done := make(chan error, 1)
	go func() {
		done <- handleConn(context.Background(), server, tr, map[string]string{"foo": "/dev/null"}, zap.NewNop())
	}()
	if _, err := client.Write([]byte("foo\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("handleConn: %v", err)
	}
	if !tr.negotiated {
		t.Fatalf("expected negotiation to occur")
	}
}
