package quic

import (
	"bytes"
	"context"
	"io"
	"testing"

	"lvmsync_go/config"
	"lvmsync_go/internal/transport"
)

func TestQUICRegistered(t *testing.T) {
	f, ok := transport.Get("quic")
	if !ok {
		t.Fatalf("quic transport not registered")
	}
	s, r, err := f(&config.Config{})
	if err != nil {
		t.Fatalf("factory error: %v", err)
	}
	if err := s.Send(context.Background(), bytes.NewReader(nil)); err != nil {
		t.Fatalf("send: %v", err)
	}
	if err := r.Receive(context.Background(), io.Discard); err != nil {
		t.Fatalf("receive: %v", err)
	}
}
