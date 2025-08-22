package ssh

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"lvmsync_go/common"
	"lvmsync_go/transport"
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
func (f failingConn) SetReadDeadline(_ time.Time) error  { return nil }
func (f failingConn) SetWriteDeadline(_ time.Time) error { return nil }

type dummyAddr struct{}

func (dummyAddr) Network() string { return "tcp" }
func (dummyAddr) String() string  { return "dummy" }

type recordingConn struct {
	net.Conn
	deadlines []time.Time
}

func (r *recordingConn) SetDeadline(t time.Time) error {
	r.deadlines = append(r.deadlines, t)
	return nil
}

func (r *recordingConn) SetReadDeadline(_ time.Time) error  { return nil }
func (r *recordingConn) SetWriteDeadline(_ time.Time) error { return nil }

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

func TestNegotiateClearDeadlineSuccess(t *testing.T) {
	tr := &Transport{logger: zap.NewNop()}
	c1, c2 := net.Pipe()
	client := &recordingConn{Conn: c1}
	server := &recordingConn{Conn: c2}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	var wg sync.WaitGroup
	var srvErr error
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, srvErr = tr.Negotiate(ctx, server, transport.Server, common.Handshake{})
	}()

	if _, err := tr.Negotiate(ctx, client, transport.Client, common.Handshake{}); err != nil {
		t.Fatalf("client negotiate: %v", err)
	}
	wg.Wait()
	if srvErr != nil {
		t.Fatalf("server negotiate: %v", srvErr)
	}
	if len(client.deadlines) == 0 {
		t.Fatalf("expected deadlines to be set")
	}
	if !client.deadlines[len(client.deadlines)-1].IsZero() {
		t.Fatalf("deadline not cleared")
	}
}
