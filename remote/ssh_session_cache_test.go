package remote

import (
	"testing"

	"go.uber.org/zap"
)

func TestSessionCloseRemovesFromCache(t *testing.T) {
	multiplexer.mu.Lock()
	multiplexer.sessions = make(map[string]*SSHSession)
	multiplexer.mu.Unlock()

	_, rawClient := newSSHServerClient(t, func(string) int { return 0 })
	client := &SSHClient{Client: rawClient, Logger: zap.NewNop()}

	const host = "cachehost"

	s1, err := GetMultiplexedSession(client, host)
	if err != nil {
		t.Fatalf("GetMultiplexedSession first: %v", err)
	}

	s2, err := GetMultiplexedSession(client, host)
	if err != nil {
		t.Fatalf("GetMultiplexedSession second: %v", err)
	}
	if s1 != s2 {
		t.Fatalf("expected cached session reuse")
	}

	s1.Close()

	multiplexer.mu.Lock()
	_, ok := multiplexer.sessions[host]
	multiplexer.mu.Unlock()
	if ok {
		t.Fatalf("session not removed from cache")
	}

	s3, err := GetMultiplexedSession(client, host)
	if err != nil {
		t.Fatalf("GetMultiplexedSession after close: %v", err)
	}
	if s3 == s1 {
		t.Fatalf("expected new session after close")
	}
	s3.Close()
}
