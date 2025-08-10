package h2

import (
	"bytes"
	"context"
	"testing"

	"go.uber.org/zap"

	"lvmsync_go/config"
	"lvmsync_go/internal/transport"
)

func TestH2Registered(t *testing.T) {
	if _, ok := transport.Get("h2"); !ok {
		t.Fatalf("h2 transport not registered")
	}
}

func TestH2SendReceive(t *testing.T) {
	sender, receiver, err := New(&config.Config{H2Port: 0}, zap.NewNop())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer receiver.(*h2Receiver).Close()

	var buf bytes.Buffer
	ctx := context.Background()
	go func() {
		if err := receiver.Receive(ctx, &buf); err != nil {
			t.Errorf("receive: %v", err)
		}
	}()
	data := []byte("hello")
	if err := sender.Send(ctx, bytes.NewReader(data)); err != nil {
		t.Fatalf("send: %v", err)
	}
	if !bytes.Equal(buf.Bytes(), data) {
		t.Fatalf("got %q want %q", buf.Bytes(), data)
	}
}
