package remote

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
)

func TestPrivilegedHelperACKNACK(t *testing.T) {
	var tmpPath string
	handler := func(_ string, ch ssh.Channel) int {
		f, err := os.CreateTemp(t.TempDir(), "priv")
		if err != nil {
			return 1
		}
		tmpPath = f.Name()
		defer f.Close() //nolint:errcheck
		if err := PrivilegedPwriteServer(ch, int(f.Fd())); err != nil && err != io.EOF {
			return 1
		}
		return 0
	}
	_, client := newSSHServerClientWithChannel(t, handler)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	privClient, err := StartPrivHelper(ctx, client, "privhelper", zap.NewNop())
	if err != nil {
		t.Fatalf("StartPrivHelper: %v", err)
	}
	defer privClient.Close() //nolint:errcheck

	if err := privClient.Send(0, []byte("hello")); err != nil {
		t.Fatalf("Send: %v", err)
	}
	badHash := sha256.Sum256([]byte("wrong"))
	if err := privClient.send(5, []byte("world"), badHash); err != nil {
		t.Fatalf("send: %v", err)
	}
	ack, err := privClient.RecvAck(ctx)
	if err != nil || !ack {
		t.Fatalf("expected ACK, got %v, err %v", ack, err)
	}
	ack, err = privClient.RecvAck(ctx)
	if err != nil || ack {
		t.Fatalf("expected NACK, got %v, err %v", ack, err)
	}
	data, err := os.ReadFile(tmpPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("unexpected file content %q", string(data))
	}
}

func TestStartPrivHelperInvalidCommand(t *testing.T) {
	_, client := newSSHServerClientWithChannel(t, func(_ string, _ ssh.Channel) int { return 0 })
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := StartPrivHelper(ctx, client, "bad;cmd", zap.NewNop()); err == nil || !strings.Contains(err.Error(), "invalid characters") {
		t.Fatalf("expected invalid characters error, got %v", err)
	}
}

func TestStartPrivHelperNilLogger(t *testing.T) {
	_, client := newSSHServerClientWithChannel(t, func(_ string, _ ssh.Channel) int { return 0 })
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := StartPrivHelper(ctx, client, "privhelper", nil); err == nil || !strings.Contains(err.Error(), "logger is nil") {
		t.Fatalf("expected nil logger error, got %v", err)
	}
}

func TestRecvAckTimeout(t *testing.T) {
	r, w := net.Pipe()
	defer r.Close() //nolint:errcheck
	defer w.Close() //nolint:errcheck
	c := &PrivHelperClient{stdout: r, logger: zap.NewNop()}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if _, err := c.RecvAck(ctx); err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context deadline exceeded, got %v", err)
	}
}

type blockingAckReader struct {
	readStarted chan struct{}
	closeCh     chan struct{}
	closed      bool
	readDone    chan struct{}
}

func newBlockingAckReader() *blockingAckReader {
	return &blockingAckReader{
		readStarted: make(chan struct{}),
		closeCh:     make(chan struct{}),
		readDone:    make(chan struct{}),
	}
}

func (r *blockingAckReader) Read(_ []byte) (int, error) {
	close(r.readStarted)
	<-r.closeCh
	close(r.readDone)
	return 0, io.EOF
}

func (r *blockingAckReader) Close() error {
	r.closed = true
	close(r.closeCh)
	return nil
}

type neverReader struct{}

func (neverReader) Read(_ []byte) (int, error) { select {} }

func TestRecvAckCancelClosesReader(t *testing.T) {
	r := newBlockingAckReader()
	c := &PrivHelperClient{stdout: r, logger: zap.NewNop()}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := c.RecvAck(ctx)
		if err == nil || !errors.Is(err, context.Canceled) {
			done <- fmt.Errorf("expected context canceled, got %v", err)
			return
		}
		done <- nil
	}()
	<-r.readStarted
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("RecvAck did not return")
	}
	if !r.closed {
		t.Fatalf("reader not closed")
	}
	select {
	case <-r.readDone:
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("read goroutine not cleaned up")
	}
}

func TestRecvAckCancelUnclosable(t *testing.T) {
	c := &PrivHelperClient{stdout: neverReader{}, logger: zap.NewNop()}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := c.RecvAck(ctx)
		if err == nil || !errors.Is(err, context.Canceled) {
			done <- fmt.Errorf("expected context canceled, got %v", err)
			return
		}
		done <- nil
	}()
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("RecvAck blocked with unclosable reader")
	}
}

type errAckReader struct {
	readStarted chan struct{}
	closeCh     chan struct{}
}

func newErrAckReader() *errAckReader {
	return &errAckReader{
		readStarted: make(chan struct{}),
		closeCh:     make(chan struct{}),
	}
}

var (
	errSetDeadline = errors.New("set deadline error")
	errClose       = errors.New("close error")
)

func (r *errAckReader) Read(_ []byte) (int, error) {
	close(r.readStarted)
	<-r.closeCh
	return 0, io.EOF
}

func (r *errAckReader) SetReadDeadline(time.Time) error { return errSetDeadline }

func (r *errAckReader) Close() error {
	close(r.closeCh)
	return errClose
}

func TestRecvAckCancelJoinErrors(t *testing.T) {
	r := newErrAckReader()
	c := &PrivHelperClient{stdout: r, logger: zap.NewNop()}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := c.RecvAck(ctx)
		done <- err
	}()
	<-r.readStarted
	cancel()
	err := <-done
	if err == nil {
		t.Fatalf("expected error")
	}
	if !errors.Is(err, context.Canceled) || !errors.Is(err, errSetDeadline) || !errors.Is(err, errClose) {
		t.Fatalf("expected joined errors, got %v", err)
	}
}

type earlyTimeoutReader struct{ deadline time.Time }

func (r *earlyTimeoutReader) Read(_ []byte) (int, error) {
	if !r.deadline.IsZero() {
		if d := time.Until(r.deadline) - 50*time.Millisecond; d > 0 {
			time.Sleep(d)
		}
	}
	return 0, &netTimeoutError{}
}

func (r *earlyTimeoutReader) SetReadDeadline(t time.Time) error {
	r.deadline = t
	return nil
}

type netTimeoutError struct{}

func (netTimeoutError) Error() string   { return "timeout" }
func (netTimeoutError) Timeout() bool   { return true }
func (netTimeoutError) Temporary() bool { return true }

func TestRecvAckNetTimeoutBeforeDeadline(t *testing.T) {
	r := &earlyTimeoutReader{}
	c := &PrivHelperClient{stdout: r, logger: zap.NewNop()}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := c.RecvAck(ctx); err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context deadline exceeded, got %v", err)
	} else {
		var ne net.Error
		if !errors.As(err, &ne) || !ne.Timeout() {
			t.Fatalf("expected timeout error, got %v", err)
		}
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		t.Fatalf("expected nil context error, got %v", ctxErr)
	}
}

func TestPrivilegedHelperOversizedLength(t *testing.T) {
	handler := func(cmd string, ch ssh.Channel) int {
		f, err := os.CreateTemp(t.TempDir(), "priv")
		if err != nil {
			return 1
		}
		defer f.Close() //nolint:errcheck
		if err := PrivilegedPwriteServer(ch, int(f.Fd())); err != nil && err != io.EOF {
			return 1
		}
		return 0
	}
	_, client := newSSHServerClientWithChannel(t, handler)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	privClient, err := StartPrivHelper(ctx, client, "privhelper", zap.NewNop())
	if err != nil {
		t.Fatalf("StartPrivHelper: %v", err)
	}
	defer privClient.Close() //nolint:errcheck

	payload := make([]byte, maxPayloadLength+1)
	if err := privClient.Send(0, payload); err != nil {
		t.Fatalf("Send: %v", err)
	}
	ack, err := privClient.RecvAck(ctx)
	if err != nil {
		if !errors.Is(err, io.EOF) {
			t.Fatalf("RecvAck: %v", err)
		}
		return
	}
	if ack {
		t.Fatalf("expected NACK for oversized payload")
	}
}

func TestPrivilegedHelperShortWrite(t *testing.T) {
	handler := func(cmd string, ch ssh.Channel) int {
		pw := func(fd int, p []byte, off int64) (int, error) {
			return len(p) - 1, nil
		}
		if err := privilegedPwriteServer(ch, 0, pw); err != nil && err != io.EOF {
			return 1
		}
		return 0
	}
	_, client := newSSHServerClientWithChannel(t, handler)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	privClient, err := StartPrivHelper(ctx, client, "privhelper", zap.NewNop())
	if err != nil {
		t.Fatalf("StartPrivHelper: %v", err)
	}
	defer privClient.Close() //nolint:errcheck

	if err := privClient.Send(0, []byte("hello")); err != nil {
		t.Fatalf("Send: %v", err)
	}
	ack, err := privClient.RecvAck(ctx)
	if err != nil {
		t.Fatalf("RecvAck: %v", err)
	}
	if ack {
		t.Fatalf("expected NACK for short write")
	}
}

type shortWriteCloser struct{}

func (shortWriteCloser) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	return len(p) - 1, nil
}

func (shortWriteCloser) Close() error { return nil }

func TestPrivHelperSendShortWrite(t *testing.T) {
	c := &PrivHelperClient{stdin: shortWriteCloser{}, logger: zap.NewNop()}
	err := c.Send(0, []byte("data"))
	if !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("expected io.ErrShortWrite, got %v", err)
	}
}

func TestPrivilegedPwriteServerLargeDeclaredLength(t *testing.T) {
	const large = 2 * 1024 * 1024 * 1024 // 2 GiB
	header := make([]byte, 8+4+32)
	binary.BigEndian.PutUint64(header[0:8], 0)
	binary.BigEndian.PutUint32(header[8:12], uint32(large))
	cr := &countingReader{limit: maxPayloadLength * 2}
	reader := io.MultiReader(bytes.NewReader(header), cr)
	rw := struct {
		io.Reader
		io.Writer
	}{Reader: reader, Writer: io.Discard}
	var called bool
	pw := func(fd int, p []byte, off int64) (int, error) {
		called = true
		return len(p), nil
	}
	err := privilegedPwriteServer(rw, 0, pw)
	if err == nil {
		t.Fatalf("expected error for large declared length")
	}
	if called {
		t.Fatalf("pwrite should not be called")
	}
	if cr.n > maxPayloadLength {
		t.Fatalf("read %d bytes, expected at most %d", cr.n, maxPayloadLength)
	}
}

func TestPrivilegedPwriteServerOversizedLengthNACK(t *testing.T) {
	payloadLen := maxPayloadLength + 1
	header := make([]byte, 8+4+32)
	binary.BigEndian.PutUint64(header[0:8], 0)
	binary.BigEndian.PutUint32(header[8:12], uint32(payloadLen))
	payload := make([]byte, payloadLen)
	reader := io.MultiReader(bytes.NewReader(header), bytes.NewReader(payload))
	var out bytes.Buffer
	rw := struct {
		io.Reader
		io.Writer
	}{Reader: reader, Writer: &out}
	pw := func(fd int, p []byte, off int64) (int, error) {
		t.Fatalf("pwrite should not be called")
		return len(p), nil
	}
	err := privilegedPwriteServer(rw, 0, pw)
	if err == nil {
		t.Fatalf("expected error for oversized payload")
	}
	if out.String() != "N" {
		t.Fatalf("expected NACK, got %q", out.String())
	}
}

type countingReader struct {
	n     int
	limit int
}

func (c *countingReader) Read(p []byte) (int, error) {
	if c.n >= c.limit {
		return 0, io.EOF
	}
	if len(p) > c.limit-c.n {
		p = p[:c.limit-c.n]
	}
	for i := range p {
		p[i] = 0
	}
	c.n += len(p)
	return len(p), nil
}
