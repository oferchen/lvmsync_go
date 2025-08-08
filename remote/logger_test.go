package remote

import (
	"testing"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"

	remotetest "lvmsync_go/remote/testutil"
)

func TestRunRemoteScriptNoLogger(t *testing.T) {
	server := remotetest.NewMockSSHServer(t, func(_ string) int { return 0 }) // cmd is unused
	defer server.Close()
	knownHosts := remotetest.CreateKnownHostsFile(t, server)
	hostKeyCallback, hostKeyErr := knownhosts.New(knownHosts)
	if hostKeyErr != nil {
		t.Fatalf("knownhosts.New: %v", hostKeyErr)
	}
	client, dialErr := ssh.Dial("tcp", server.Addr, &ssh.ClientConfig{User: "test", HostKeyCallback: hostKeyCallback})
	if dialErr != nil {
		t.Fatalf("failed to dial: %v", dialErr)
	}
	defer client.Close() //nolint:errcheck

	SetLogger(nil)

	script := "echo hi"
	if scriptErr := RunRemoteScript(client, script); scriptErr != nil {
		t.Fatalf("RunRemoteScript error: %v", scriptErr)
	}
}

func TestSendKeepAliveNoLogger(t *testing.T) {
	server := remotetest.NewMockSSHServer(t, func(_ string) int { return 0 }) // cmd is unused
	defer server.Close()
	knownHosts := remotetest.CreateKnownHostsFile(t, server)
	hostKeyCallback, hostKeyErr := knownhosts.New(knownHosts)
	if hostKeyErr != nil {
		t.Fatalf("knownhosts.New: %v", hostKeyErr)
	}
	client, dialErr := ssh.Dial("tcp", server.Addr, &ssh.ClientConfig{User: "test", HostKeyCallback: hostKeyCallback})
	if dialErr != nil {
		t.Fatalf("failed to dial: %v", dialErr)
	}
	defer client.Close() //nolint:errcheck

	SetLogger(nil)

	if keepErr := sendKeepAlive(client, "host"); keepErr != nil {
		t.Fatalf("sendKeepAlive error: %v", keepErr)
	}
}
