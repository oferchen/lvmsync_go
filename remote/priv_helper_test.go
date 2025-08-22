package remote

import (
	"context"
	"crypto/sha256"
	"errors"
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

func TestRecvAckTimeout(t *testing.T) {
	r, w := net.Pipe()
	defer r.Close() //nolint:errcheck
	defer w.Close() //nolint:errcheck
	c := &PrivHelperClient{stdout: r}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if _, err := c.RecvAck(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)

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
	ack, err := privClient.RecvAck()
	if err != nil {
		t.Fatalf("RecvAck: %v", err)
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
	ack, err := privClient.RecvAck()
	if err != nil {
		t.Fatalf("RecvAck: %v", err)
	}
	if ack {
		t.Fatalf("expected NACK for short write")

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
