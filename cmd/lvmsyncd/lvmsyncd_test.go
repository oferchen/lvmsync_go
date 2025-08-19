package main

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"

	rootcmd "lvmsync_go/cmd/root"
	"lvmsync_go/common"
	"lvmsync_go/internal/exitcode"
	"lvmsync_go/transport"
)

// mockTransport implements transport.Interface for testing negotiation.
type mockTransport struct {
	negotiated bool
	listened   *[]string
	mu         sync.Mutex
}

func (m *mockTransport) Name() string                                   { return "mock" }
func (m *mockTransport) Dial(context.Context, string) (net.Conn, error) { return nil, nil }
func (m *mockTransport) Listen(ctx context.Context, addr string) (net.Listener, error) {
	if m.listened != nil {
		m.mu.Lock()
		*m.listened = append(*m.listened, addr)
		m.mu.Unlock()
	}
	return &dummyListener{ctx: ctx, addr: dummyAddr(addr)}, nil
}
func (m *mockTransport) Negotiate(context.Context, net.Conn, transport.Role, common.Handshake) (common.Handshake, error) {
	m.negotiated = true
	return common.Handshake{}, nil
}

type dummyListener struct {
	ctx  context.Context
	addr net.Addr
}

func (d *dummyListener) Accept() (net.Conn, error) {
	<-d.ctx.Done()
	return nil, d.ctx.Err()
}
func (d *dummyListener) Close() error   { return nil }
func (d *dummyListener) Addr() net.Addr { return d.addr }

func TestFlagParsing(t *testing.T) {
	v := viper.New()
	cmd := &cobra.Command{}
	if err := bindFlags(cmd, v); err != nil {
		t.Fatalf("bindFlags: %v", err)
	}
	args := []string{"--module", "/dev/null", "--module", "/dev/zero", "--listen", "h2://:9000", "--listen", "tcp+tls://:9443"}
	if err := cmd.ParseFlags(args); err != nil {
		t.Fatalf("ParseFlags: %v", err)
	}
	opts := parseOptions(v)
	if len(opts.modules) != 2 || opts.listens[0] != "h2://:9000" || opts.listens[1] != "tcp+tls://:9443" {
		t.Fatalf("options parsed incorrectly: %+v", opts)
	}
}

func TestModuleACL(t *testing.T) {
	tr := &mockTransport{}
	server, client := net.Pipe()
	done := make(chan error, 1)
	go func() {
		done <- handleConn(context.Background(), server, tr, map[string]struct{}{"/dev/null": {}}, zap.NewNop())
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
		done <- handleConn(context.Background(), server, tr, map[string]struct{}{"/dev/null": {}}, zap.NewNop())
	}()
	if _, err := client.Write([]byte("/dev/null\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("handleConn: %v", err)
	}
	if !tr.negotiated {
		t.Fatalf("expected negotiation to occur")
	}
}

func TestListenerSetup(t *testing.T) {
	var addrs []string
	if err := transport.Register("mocklisten", func(cfg transport.Config) (transport.Interface, error) {
		return &mockTransport{listened: &addrs}, nil
	}); err != nil && !strings.Contains(err.Error(), "already registered") {
		t.Fatalf("register: %v", err)
	}
	opts := options{
		modules:       map[string]struct{}{"/dev/null": {}},
		listens:       []string{"mocklisten://:1111", "mocklisten://:2222"},
		allowInsecure: true,
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- start(ctx, opts, zap.NewNop()) }()
	time.Sleep(100 * time.Millisecond)
	cancel()
	err := <-done
	if err != context.Canceled {
		t.Fatalf("start returned %v", err)
	}
	if len(addrs) != 2 || addrs[0] != ":1111" || addrs[1] != ":2222" {
		t.Fatalf("listener addresses %v", addrs)
	}
}

func TestExitCodeMapping(t *testing.T) {
	tests := []struct {
		name string
		err  error
		code int
	}{
		{"success", nil, exitcode.OK},
		{"config", errors.New("parse listen"), exitcode.ErrConfig},
		{"runtime", errors.New("listen failed"), exitcode.ErrRuntime},
		{"verify", errors.New("digest mismatch"), exitcode.ErrVerify},
		{"partial", errors.New("received signal: interrupt"), exitcode.ErrPartial},
		{"precondition", errors.New("precondition not met"), exitcode.ErrPrecondition},
		{"resumable", errors.New("resumable: retry later"), exitcode.ErrResumable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if c := rootcmd.ExitCode(tt.err); c != tt.code {
				t.Fatalf("expected %d, got %d", tt.code, c)
			}
		})
	}
}

func TestRunNilLoggerError(t *testing.T) {
	if err := run([]string{}, nil); err == nil {
		t.Fatalf("expected error")
	}
}
