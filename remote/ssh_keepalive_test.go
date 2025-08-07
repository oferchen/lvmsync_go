package remote

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"net"
	"os"
	"strconv"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

func TestSendKeepAlive(t *testing.T) {
	server := newMockSSHServer(t, func(cmd string) int { return 0 })
	defer server.Close()
	knownHosts := createKnownHostsFile(t, server)
	hostKeyCallback, err := knownhosts.New(knownHosts)
	if err != nil {
		t.Fatalf("knownhosts.New: %v", err)
	}
	client, err := ssh.Dial("tcp", server.addr, &ssh.ClientConfig{User: "test", HostKeyCallback: hostKeyCallback})
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}
	defer client.Close() //nolint:errcheck

	if err := sendKeepAlive(client, "host"); err != nil {
		t.Fatalf("sendKeepAlive error: %v", err)
	}

	reqs := server.GlobalRequests()
	if len(reqs) != 1 || reqs[0] != "keepalive@openssh.com" {
		t.Fatalf("expected one keepalive request, got %v", reqs)
	}
}

func TestStartKeepAlive(t *testing.T) {
	server := newMockSSHServer(t, func(cmd string) int { return 0 })
	defer server.Close()
	knownHosts := createKnownHostsFile(t, server)
	hostKeyCallback, err := knownhosts.New(knownHosts)
	if err != nil {
		t.Fatalf("knownhosts.New: %v", err)
	}
	client, err := ssh.Dial("tcp", server.addr, &ssh.ClientConfig{User: "test", HostKeyCallback: hostKeyCallback})
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}

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

func createTempKey(t *testing.T) string {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	f, err := os.CreateTemp(t.TempDir(), "id_rsa")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	if _, err := f.Write(keyPEM); err != nil {
		t.Fatalf("write key: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close key: %v", err)
	}
	return f.Name()
}

func TestNewSSHClient(t *testing.T) {
	server := newMockSSHServer(t, func(cmd string) int { return 0 })
	defer server.Close()

	host, portStr, errSplit := net.SplitHostPort(server.addr)
	if errSplit != nil {
		t.Fatalf("SplitHostPort: %v", errSplit)
	}
	port, errConv := strconv.Atoi(portStr)
	if errConv != nil {
		t.Fatalf("Atoi: %v", errConv)
	}

	keyPath := createTempKey(t)
	knownHosts := createKnownHostsFile(t, server)
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
	server := newMockSSHServer(t, func(cmd string) int { return 0 })
	defer server.Close()

	keyPath := createTempKey(t)
	knownHosts := createKnownHostsFile(t, server)
	mgr, err := NewSSHManager("test", keyPath, time.Second, knownHosts)
	if err != nil {
		t.Fatalf("NewSSHManager error: %v", err)
	}

	host, portStr, errSplit := net.SplitHostPort(server.addr)
	if errSplit != nil {
		t.Fatalf("SplitHostPort: %v", errSplit)
	}
	port, errConv := strconv.Atoi(portStr)
	if errConv != nil {
		t.Fatalf("Atoi: %v", errConv)
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

	mgr.CloseAll()

	client3, err := mgr.GetClient(host, port)
	if err != nil {
		t.Fatalf("GetClient after CloseAll error: %v", err)
	}
	if client3 == client1 {
		t.Fatalf("expected new client after CloseAll")
	}
	waitForConnectionCount(t, server, 2)

	mgr.CloseAll()
}

func waitForConnectionCount(t *testing.T, server *mockSSHServer, expected int) {
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
