package quic

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/oferchen/lvmsync_go/common"
	"github.com/oferchen/lvmsync_go/transport"
)

type failingConn struct{}

func (f failingConn) Read(b []byte) (int, error)  { return 0, io.EOF }
func (f failingConn) Write(b []byte) (int, error) { return 0, io.ErrClosedPipe }
func (f failingConn) Close() error                { return nil }
func (f failingConn) LocalAddr() net.Addr         { return dummyAddr{} }
func (f failingConn) RemoteAddr() net.Addr        { return dummyAddr{} }
func (f failingConn) SetDeadline(t time.Time) error {
	if t.IsZero() {
		return errors.New("boom")
	}
	return nil
}
func (f failingConn) SetReadDeadline(t time.Time) error  { return nil }
func (f failingConn) SetWriteDeadline(t time.Time) error { return nil }

type dummyAddr struct{}

func (dummyAddr) Network() string { return "tcp" }
func (dummyAddr) String() string  { return "dummy" }

func TestNegotiateClearDeadlineFailure(t *testing.T) {
	core, logs := observer.New(zapcore.InfoLevel)
	tr := &Transport{logger: zap.New(core)}
	conn := failingConn{}
	if _, err := tr.Negotiate(context.Background(), conn, transport.Client, common.Handshake{}); err == nil {
		t.Fatalf("expected negotiate error")
	}
	entries := logs.FilterMessage("clear_deadline_failed").All()
	if len(entries) != 1 {
		t.Fatalf("expected 1 clear_deadline_failed log, got %d", len(entries))
	}
	if entries[0].Level != zapcore.ErrorLevel {
		t.Fatalf("expected error level, got %v", entries[0].Level)
	}
}
