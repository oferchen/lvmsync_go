package remote

import (
	"testing"

	"go.uber.org/zap"
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

	sshClient := &SSHClient{Client: client, Logger: zap.NewNop()}

	script := "echo hi"
	if scriptErr := sshClient.RunRemoteScript(script); scriptErr != nil {
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

	sshClient := &SSHClient{Client: client, Logger: zap.NewNop()}

	if keepErr := sshClient.sendKeepAlive("host"); keepErr != nil {
		t.Fatalf("sendKeepAlive error: %v", keepErr)
	}
}
