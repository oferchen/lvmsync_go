package remote

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"

	remotetest "lvmsync_go/remote/testutil"
)

func TestSendKeepAlive(t *testing.T) {
	server, rawClient := newSSHServerClient(t, func(_ string) int { return 0 }) // cmd is unused

	client := &SSHClient{Client: rawClient, Logger: zap.NewNop()}

	if err := client.sendKeepAlive("host"); err != nil {
		t.Fatalf("sendKeepAlive error: %v", err)
	}

	reqs := server.GlobalRequests()
	if len(reqs) != 1 || reqs[0] != "keepalive@openssh.com" {
		t.Fatalf("expected one keepalive request, got %v", reqs)
	}
}

func TestStartKeepAlive(t *testing.T) {
	server, rawClient := newSSHServerClient(t, func(_ string) int { return 0 }) // cmd is unused

	client := &SSHClient{Client: rawClient, Logger: zap.NewNop()}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	done := make(chan struct{})
	go func() {
		client.startKeepAlive(ctx, "host", 10*time.Millisecond)
		close(done)
	}()

	time.Sleep(30 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatalf("startKeepAlive did not exit")
	}
	if err := client.Close(); err != nil {
		t.Fatalf("client.Close error: %v", err)
	}

	if len(server.GlobalRequests()) == 0 {
		t.Fatalf("expected keepalive requests, got none")
	}
}

func TestNewSSHClient(t *testing.T) {
	server, host, port, knownHosts := newSSHServer(t, func(_ string) int { return 0 }) // cmd is unused
	keyPath := remotetest.CreateTempKey(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	client, err := NewSSHClient(ctx, host, "test", keyPath, port, knownHosts, true, time.Second, 10*time.Millisecond, 0, zap.NewNop())
	if err != nil {
		t.Fatalf("NewSSHClient error: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	cancel()
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
	initCtx, initCancel := context.WithTimeout(context.Background(), time.Second)
	defer initCancel()
	mgr, err := NewSSHManager(initCtx, "test", keyPath, time.Second, knownHosts, zap.NewNop())
	if err != nil {
		t.Fatalf("NewSSHManager error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	client1, err := mgr.GetClient(ctx, host, port)
	if err != nil {
		t.Fatalf("GetClient error: %v", err)
	}

	client2, err := mgr.GetClient(ctx, host, port)
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

	ctx2, cancel2 := context.WithTimeout(context.Background(), time.Second)
	defer cancel2()
	client3, err := mgr.GetClient(ctx2, host, port)
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
