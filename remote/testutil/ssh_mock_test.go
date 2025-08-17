package testutil

import (
	"bytes"
	"net"
	"os"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

func TestMockSSHServerRecordsCommands(t *testing.T) {
	srv := NewMockSSHServer(t, func(cmd string) int { return 0 })
	defer srv.Close()
	client, err := ssh.Dial("tcp", srv.Addr, &ssh.ClientConfig{
		User:            "test",
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Auth:            []ssh.AuthMethod{ssh.Password("")},
	})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()
	sess, err := client.NewSession()
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	defer sess.Close()
	var b bytes.Buffer
	sess.Stdout = &b
	if err := sess.Run("echo hello"); err != nil {
		t.Fatalf("run: %v", err)
	}
	cmds := srv.Commands()
	if len(cmds) != 1 || cmds[0] != "echo hello" {
		t.Fatalf("unexpected commands %v", cmds)
	}
}

func TestCreateKnownHostsFile(t *testing.T) {
	srv := NewMockSSHServer(t, func(cmd string) int { return 0 })
	defer srv.Close()
	path := CreateKnownHostsFile(t, srv)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	host, port, err := net.SplitHostPort(srv.Addr)
	if err != nil {
		t.Fatalf("SplitHostPort: %v", err)
	}
	expected := "[" + host + "]:" + port
	if !strings.Contains(string(data), expected) {
		t.Fatalf("missing host entry")
	}
}
