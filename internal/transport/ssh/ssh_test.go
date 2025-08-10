package ssh

import (
	"bytes"
	"context"
	"io"
	"testing"

	"lvmsync_go/config"
	"lvmsync_go/internal/transport"
)

func TestSSHRegistered(t *testing.T) {
	f, ok := transport.Get("ssh")
	if !ok {
		t.Fatalf("ssh transport not registered")
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
