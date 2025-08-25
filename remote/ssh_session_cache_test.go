package remote

import (
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestSessionCloseRemovesFromCache(t *testing.T) {
	multiplexer.mu.Lock()
	multiplexer.sessions = make(map[string]*sessionEntry)
	multiplexer.ttl = time.Minute
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

func TestSessionExpiresFromCache(t *testing.T) {
	multiplexer.mu.Lock()
	multiplexer.sessions = make(map[string]*sessionEntry)
	multiplexer.ttl = 50 * time.Millisecond
	multiplexer.mu.Unlock()

	_, rawClient := newSSHServerClient(t, func(string) int { return 0 })
	client := &SSHClient{Client: rawClient, Logger: zap.NewNop()}

	const host = "expirehost"

	s, err := GetMultiplexedSession(client, host)
	if err != nil {
		t.Fatalf("GetMultiplexedSession: %v", err)
	}

	time.Sleep(2 * multiplexer.ttl)

	multiplexer.mu.Lock()
	_, ok := multiplexer.sessions[host]
	multiplexer.mu.Unlock()
	if ok {
		t.Fatalf("session did not expire")
	}

	// session should already be closed, but ensure no panic when calling Close
	s.Close()
}

func TestCloseAllSessions(t *testing.T) {
	multiplexer.mu.Lock()
	multiplexer.sessions = make(map[string]*sessionEntry)
	multiplexer.ttl = time.Minute
	multiplexer.mu.Unlock()

	_, rawClient := newSSHServerClient(t, func(string) int { return 0 })
	client := &SSHClient{Client: rawClient, Logger: zap.NewNop()}

	const host1 = "host1"
	const host2 = "host2"

	if _, err := GetMultiplexedSession(client, host1); err != nil {
		t.Fatalf("GetMultiplexedSession host1: %v", err)
	}
	if _, err := GetMultiplexedSession(client, host2); err != nil {
		t.Fatalf("GetMultiplexedSession host2: %v", err)
	}

	CloseAllSessions()

	multiplexer.mu.Lock()
	if len(multiplexer.sessions) != 0 {
		multiplexer.mu.Unlock()
		t.Fatalf("expected all sessions closed")
	}
	multiplexer.mu.Unlock()
}
