package quic

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	quic "github.com/quic-go/quic-go"
)

// stubQConn implements quic.Connection for testing Conn.Close.
type stubQConn struct {
	closeErr   error
	localAddr  net.Addr
	remoteAddr net.Addr
}

func (s *stubQConn) AcceptStream(context.Context) (quic.Stream, error)           { return nil, nil }
func (s *stubQConn) AcceptUniStream(context.Context) (quic.ReceiveStream, error) { return nil, nil }
func (s *stubQConn) OpenStream() (quic.Stream, error)                            { return nil, nil }
func (s *stubQConn) OpenStreamSync(context.Context) (quic.Stream, error)         { return nil, nil }
func (s *stubQConn) OpenUniStream() (quic.SendStream, error)                     { return nil, nil }
func (s *stubQConn) OpenUniStreamSync(context.Context) (quic.SendStream, error)  { return nil, nil }
func (s *stubQConn) LocalAddr() net.Addr                                         { return s.localAddr }
func (s *stubQConn) RemoteAddr() net.Addr                                        { return s.remoteAddr }
func (s *stubQConn) CloseWithError(quic.ApplicationErrorCode, string) error      { return s.closeErr }
func (s *stubQConn) Context() context.Context                                    { return context.Background() }
func (s *stubQConn) ConnectionState() quic.ConnectionState                       { return quic.ConnectionState{} }
func (s *stubQConn) SendDatagram([]byte) error                                   { return nil }
func (s *stubQConn) ReceiveDatagram(context.Context) ([]byte, error)             { return nil, nil }

// stubStream implements quic.Stream for testing.
type stubStream struct{ closeErr error }

func (s *stubStream) StreamID() quic.StreamID          { return 0 }
func (s *stubStream) Read([]byte) (int, error)         { return 0, io.EOF }
func (s *stubStream) Write(p []byte) (int, error)      { return len(p), nil }
func (s *stubStream) Close() error                     { return s.closeErr }
func (s *stubStream) CancelRead(quic.StreamErrorCode)  {}
func (s *stubStream) CancelWrite(quic.StreamErrorCode) {}
func (s *stubStream) Context() context.Context         { return context.Background() }
func (s *stubStream) SetDeadline(time.Time) error      { return nil }
func (s *stubStream) SetReadDeadline(time.Time) error  { return nil }
func (s *stubStream) SetWriteDeadline(time.Time) error { return nil }

func TestConnClose(t *testing.T) {
	addr := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 1}
	qerr := errors.New("qconn close")
	serr := errors.New("stream close")

	tests := []struct {
		name     string
		qcErr    error
		stErr    error
		wantErrs []error
		wantLogs bool
	}{
		{"success", nil, nil, nil, false},
		{"qconn_error", qerr, nil, []error{qerr}, true},
		{"stream_error", nil, serr, []error{serr}, true},
		{"both_error", qerr, serr, []error{qerr, serr}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core, obs := observer.New(zap.DebugLevel)
			c := &Conn{
				qconn:  &stubQConn{closeErr: tt.qcErr, localAddr: addr, remoteAddr: addr},
				stream: &stubStream{closeErr: tt.stErr},
				logger: zap.New(core),
			}
			err := c.Close()
			if len(tt.wantErrs) == 0 {
				if err != nil {
					t.Fatalf("expected nil error, got %v", err)
				}
				if obs.Len() != 0 {
					t.Fatalf("expected no logs, got %d", obs.Len())
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error")
			}
			for _, we := range tt.wantErrs {
				if !errors.Is(err, we) {
					t.Fatalf("expected error %v, got %v", we, err)
				}
			}
			if obs.Len() != 1 {
				t.Fatalf("expected 1 log entry, got %d", obs.Len())
			}
			if obs.All()[0].Message != "close_failed" {
				t.Fatalf("unexpected log message %q", obs.All()[0].Message)
			}
		})
	}
}
