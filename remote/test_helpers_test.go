package remote

import (
	"net"
	"strconv"
	"testing"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"

	remotetest "github.com/oferchen/lvmsync_go/remote/testutil"
)

// newSSHServer sets up a mock SSH server and returns the server instance,
// host, port, and path to a known_hosts file containing its key.
func newSSHServer(t *testing.T, handler func(string) int) (*remotetest.MockSSHServer, string, int, string) {
	t.Helper()
	server := remotetest.NewMockSSHServer(t, handler)
	host, portStr, err := net.SplitHostPort(server.Addr)
	if err != nil {
		t.Fatalf("SplitHostPort: %v", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("Atoi: %v", err)
	}
	knownHosts := remotetest.CreateKnownHostsFile(t, server)
	t.Cleanup(func() { server.Close() })
	return server, host, port, knownHosts
}

// newSSHServerClient returns a mock SSH server and connected client.
func newSSHServerClient(t *testing.T, handler func(string) int) (*remotetest.MockSSHServer, *ssh.Client) {
	t.Helper()
	server, _, _, knownHosts := newSSHServer(t, handler)
	hostKeyCallback, err := knownhosts.New(knownHosts)
	if err != nil {
		t.Fatalf("knownhosts.New: %v", err)
	}
	client, err := ssh.Dial("tcp", server.Addr, &ssh.ClientConfig{User: "test", HostKeyCallback: hostKeyCallback})
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}
	t.Cleanup(func() {
		client.Close() //nolint:errcheck
		server.Close()
	})
	return server, client
}

// newSSHServerWithChannel sets up a mock SSH server with a channel-aware handler.
func newSSHServerWithChannel(t *testing.T, handler remotetest.ExecHandler) (*remotetest.MockSSHServer, string, int, string) {
	t.Helper()
	server := remotetest.NewMockSSHServerWithChannel(t, handler)
	host, portStr, err := net.SplitHostPort(server.Addr)
	if err != nil {
		t.Fatalf("SplitHostPort: %v", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("Atoi: %v", err)
	}
	knownHosts := remotetest.CreateKnownHostsFile(t, server)
	t.Cleanup(func() { server.Close() })
	return server, host, port, knownHosts
}

// newSSHServerClientWithChannel returns a mock SSH server and connected client using a channel-aware handler.
func newSSHServerClientWithChannel(t *testing.T, handler remotetest.ExecHandler) (*remotetest.MockSSHServer, *ssh.Client) {
	t.Helper()
	server, _, _, knownHosts := newSSHServerWithChannel(t, handler)
	hostKeyCallback, err := knownhosts.New(knownHosts)
	if err != nil {
		t.Fatalf("knownhosts.New: %v", err)
	}
	client, err := ssh.Dial("tcp", server.Addr, &ssh.ClientConfig{User: "test", HostKeyCallback: hostKeyCallback})
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}
	t.Cleanup(func() {
		client.Close() //nolint:errcheck
		server.Close()
	})
	return server, client
}
