package remote

import (
	"testing"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

func TestRunRemoteScriptNoLogger(t *testing.T) {
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

	SetLogger(nil)

	script := "echo hi"
	if err := RunRemoteScript(client, script); err != nil {
		t.Fatalf("RunRemoteScript error: %v", err)
	}
}

func TestSendKeepAliveNoLogger(t *testing.T) {
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

	SetLogger(nil)

	if err := sendKeepAlive(client, "host"); err != nil {
		t.Fatalf("sendKeepAlive error: %v", err)
	}
}
