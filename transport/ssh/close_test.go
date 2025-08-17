package ssh

import (
	"bytes"
	"errors"
	"io"
	"net"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	"golang.org/x/crypto/ssh"
)

// mockConn implements ssh.Conn for testing Close error aggregation.
type mockConn struct{ closeErr error }

func (m *mockConn) SendRequest(string, bool, []byte) (bool, []byte, error) { panic("not implemented") }
func (m *mockConn) OpenChannel(string, []byte) (ssh.Channel, <-chan *ssh.Request, error) {
	panic("not implemented")
}
func (m *mockConn) Close() error          { return m.closeErr }
func (m *mockConn) Wait() error           { return m.closeErr }
func (m *mockConn) User() string          { return "" }
func (m *mockConn) SessionID() []byte     { return nil }
func (m *mockConn) ClientVersion() []byte { return nil }
func (m *mockConn) ServerVersion() []byte { return nil }
func (m *mockConn) RemoteAddr() net.Addr  { return &net.IPAddr{} }
func (m *mockConn) LocalAddr() net.Addr   { return &net.IPAddr{} }

// mockChannel implements ssh.Channel with controllable Close error.
type mockChannel struct{ closeErr error }

func (m *mockChannel) Read([]byte) (int, error)                       { return 0, io.EOF }
func (m *mockChannel) Write(p []byte) (int, error)                    { return len(p), nil }
func (m *mockChannel) Close() error                                   { return m.closeErr }
func (m *mockChannel) CloseWrite() error                              { return nil }
func (m *mockChannel) SendRequest(string, bool, []byte) (bool, error) { return false, nil }
func (m *mockChannel) Stderr() io.ReadWriter                          { return bytes.NewBuffer(nil) }

func TestSSHConnCloseAggregatesChannelError(t *testing.T) {
	clientErr := errors.New("client close")
	chErr := errors.New("channel close")
	c1, c2 := net.Pipe()
	defer c2.Close()
	sc := &sshConn{
		netConn: c1,
		channel: &mockChannel{closeErr: chErr},
		client:  &ssh.Client{Conn: &mockConn{closeErr: clientErr}},
		logger:  zap.NewNop(),
	}
	if err := sc.Close(); !errors.Is(err, clientErr) || !errors.Is(err, chErr) {
		t.Fatalf("expected aggregated error, got %v", err)
	}
}

func TestServerConnCloseAggregatesChannelError(t *testing.T) {
	srvErr := errors.New("server close")
	chErr := errors.New("channel close")
	c1, c2 := net.Pipe()
	defer c2.Close()
	core, logs := observer.New(zap.InfoLevel)
	s := &serverConn{
		sshConn: &ssh.ServerConn{Conn: &mockConn{closeErr: srvErr}},
		netConn: c1,
		channel: &mockChannel{closeErr: chErr},
		logger:  zap.New(core),
	}
	if err := s.Close(); !errors.Is(err, srvErr) || !errors.Is(err, chErr) {
		t.Fatalf("expected aggregated error, got %v", err)
	}
	entries := logs.FilterMessage("close_end").All()
	if len(entries) != 1 {
		t.Fatalf("expected close_end log, got %d", len(entries))
	}
	if entries[0].Level != zapcore.ErrorLevel {
		t.Fatalf("expected error level, got %v", entries[0].Level)
	}
	if _, ok := entries[0].ContextMap()["error"]; !ok {
		t.Fatalf("expected error field in log")
	}
}
