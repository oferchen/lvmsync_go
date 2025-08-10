package tcp

import (
	"bytes"
	"context"
	"io"
	"sync"
	"testing"

	"lvmsync_go/config"
	"lvmsync_go/internal/transport"
)

func TestTCPRegistered(t *testing.T) {
	if _, ok := transport.Get("tcp+tls"); !ok {
		t.Fatalf("tcp+tls transport not registered")
	}
}

func TestTCPSendReceive(t *testing.T) {
	s, r, err := New(&config.Config{TCPPort: 0})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer r.(*tcpReceiver).Close()

	data := []byte("hello world")
	var buf bytes.Buffer
	ctx := context.Background()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := r.Receive(ctx, &buf); err != nil {
			t.Errorf("receive: %v", err)
		}
	}()
	if err := s.Send(ctx, bytes.NewReader(data)); err != nil {
		t.Fatalf("send: %v", err)
	}
	wg.Wait()
	if !bytes.Equal(buf.Bytes(), data) {
		t.Fatalf("got %q want %q", buf.Bytes(), data)
	}
}

func TestTCPSendError(t *testing.T) {
	s, r, err := New(&config.Config{TCPPort: 0})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	r.(*tcpReceiver).Close()
	if err := s.Send(context.Background(), bytes.NewReader([]byte("x"))); err == nil {
		t.Fatalf("expected error")
	}
}

func TestTCPReceiveCanceled(t *testing.T) {
	_, r, err := New(&config.Config{TCPPort: 0})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := r.Receive(ctx, io.Discard); err == nil {
		t.Fatalf("expected error")
	}
	r.(*tcpReceiver).Close()
}
