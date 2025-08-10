package tcp

import (
	"bytes"
	"context"
	"sync"
	"testing"

	"go.uber.org/zap"

	"lvmsync_go/config"
	"lvmsync_go/internal/transport"
)

func TestTCPRegistered(t *testing.T) {
	if _, ok := transport.Get("tcp+tls"); !ok {
		t.Fatalf("tcp+tls transport not registered")
	}
}

func TestTCPSendReceive(t *testing.T) {
	sender, receiver, err := New(&config.Config{TCPPort: 0}, zap.NewNop())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer receiver.(*tcpReceiver).Close()

	var buf bytes.Buffer
	ctx := context.Background()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := receiver.Receive(ctx, &buf); err != nil {
			t.Errorf("receive: %v", err)
		}
	}()
	data := []byte("hello")
	if err := sender.Send(ctx, bytes.NewReader(data)); err != nil {
		t.Fatalf("send: %v", err)
	}
	wg.Wait()
	if !bytes.Equal(buf.Bytes(), data) {
		t.Fatalf("got %q want %q", buf.Bytes(), data)
	}
}
