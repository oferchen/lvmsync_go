package remote

import (
	"testing"
	"time"

	remotetest "lvmsync_go/remote/testutil"
)

func TestSendKeepAlive(t *testing.T) {
	server, client := newSSHServerClient(t, func(_ string) int { return 0 }) // cmd is unused

	if err := sendKeepAlive(client, "host"); err != nil {
		t.Fatalf("sendKeepAlive error: %v", err)
	}

	reqs := server.GlobalRequests()
	if len(reqs) != 1 || reqs[0] != "keepalive@openssh.com" {
		t.Fatalf("expected one keepalive request, got %v", reqs)
	}
}

func TestStartKeepAlive(t *testing.T) {
	server, client := newSSHServerClient(t, func(_ string) int { return 0 }) // cmd is unused

	done := make(chan struct{})
	go func() {
		startKeepAlive(client, "host", 10*time.Millisecond)
		close(done)
	}()

	time.Sleep(30 * time.Millisecond)
	if err := client.Close(); err != nil {
		t.Fatalf("client.Close error: %v", err)
	}

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatalf("startKeepAlive did not exit")
	}

	if len(server.GlobalRequests()) == 0 {
		t.Fatalf("expected keepalive requests, got none")
	}
}

func TestNewSSHClient(t *testing.T) {
	server, host, port, knownHosts := newSSHServer(t, func(_ string) int { return 0 }) // cmd is unused
	keyPath := remotetest.CreateTempKey(t)
	client, err := NewSSHClient(host, "test", keyPath, port, knownHosts, true, time.Second, 10*time.Millisecond, 0)
	if err != nil {
		t.Fatalf("NewSSHClient error: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	if err := client.Close(); err != nil {
		t.Fatalf("client.Close error: %v", err)
	}

	if len(server.GlobalRequests()) == 0 {
		t.Fatalf("expected keepalive requests, got none")
	}
}

func TestSSHManager(t *testing.T) {
	server, host, port, knownHosts := newSSHServer(t, func(_ string) int { return 0 }) // cmd is unused
	keyPath := remotetest.CreateTempKey(t)
	mgr, err := NewSSHManager("test", keyPath, time.Second, knownHosts)
	if err != nil {
		t.Fatalf("NewSSHManager error: %v", err)
	}

	client1, err := mgr.GetClient(host, port)
	if err != nil {
		t.Fatalf("GetClient error: %v", err)
	}

	client2, err := mgr.GetClient(host, port)
	if err != nil {
		t.Fatalf("GetClient second error: %v", err)
	}
	if client1 != client2 {
		t.Fatalf("expected cached client")
	}
	waitForConnectionCount(t, server, 1)

	if err = mgr.CloseAll(); err != nil {
		t.Fatalf("CloseAll error: %v", err)
	}

	client3, err := mgr.GetClient(host, port)
	if err != nil {
		t.Fatalf("GetClient after CloseAll error: %v", err)
	}
	if client3 == client1 {
		t.Fatalf("expected new client after CloseAll")
	}
	waitForConnectionCount(t, server, 2)

	if err = mgr.CloseAll(); err != nil {
		t.Fatalf("CloseAll error: %v", err)
	}
}

func waitForConnectionCount(t *testing.T, server *remotetest.MockSSHServer, expected int) {
	t.Helper()
	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		if server.ConnectionCount() == expected {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("expected %d connections, got %d", expected, server.ConnectionCount())
}
