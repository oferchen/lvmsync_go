package h2

import (
	"bytes"
	"context"
	"testing"
	"time"

	"lvmsync_go/config"
	"lvmsync_go/internal/transport"
)

func TestH2Registered(t *testing.T) {
	if _, ok := transport.Get("h2"); !ok {
		t.Fatalf("h2 transport not registered")
	}
}

func TestH2Transfer(t *testing.T) {
	f, _ := transport.Get("h2")
	cfg := &config.Config{H2Port: 0}
	sender, receiver, err := f(cfg)
	if err != nil {
		t.Fatalf("factory error: %v", err)
	}
	data := []byte("hello h2")
	var buf bytes.Buffer
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- receiver.Receive(ctx, &buf) }()
	if err := sender.Send(ctx, bytes.NewReader(data)); err != nil {
		t.Fatalf("send: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("receive: %v", err)
	}
	if !bytes.Equal(buf.Bytes(), data) {
		t.Fatalf("got %q want %q", buf.Bytes(), data)
	}
}

func TestH2ContextCancel(t *testing.T) {
	f, _ := transport.Get("h2")
	cfg := &config.Config{H2Port: 0}
	sender, receiver, err := f(cfg)
	if err != nil {
		t.Fatalf("factory error: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var buf bytes.Buffer
	rctx, rcancel := context.WithTimeout(context.Background(), time.Second)
	errCh := make(chan error, 1)
	go func() { errCh <- receiver.Receive(rctx, &buf) }()
	if err := sender.Send(ctx, bytes.NewReader([]byte("cancel"))); err == nil {
		t.Fatalf("expected error on canceled context")
	}
	rcancel()
	<-errCh
}
