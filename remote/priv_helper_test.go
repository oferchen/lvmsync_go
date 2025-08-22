package remote

import (
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
)

func TestPrivilegedHelperACKNACK(t *testing.T) {
	var tmpPath string
	handler := func(cmd string, ch ssh.Channel) int {
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
	ack, err := privClient.RecvAck()
	if err != nil || !ack {
		t.Fatalf("expected ACK, got %v, err %v", ack, err)
	}
	ack, err = privClient.RecvAck()
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
	_, client := newSSHServerClientWithChannel(t, func(cmd string, ch ssh.Channel) int { return 0 })
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := StartPrivHelper(ctx, client, "bad;cmd", zap.NewNop()); err == nil || !strings.Contains(err.Error(), "invalid characters") {
		t.Fatalf("expected invalid characters error, got %v", err)
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
