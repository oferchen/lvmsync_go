package remote

import (
	"crypto/rand"
	"crypto/rsa"
	"net"
	"strings"
	"sync"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"golang.org/x/crypto/ssh"
)

type mockSSHServer struct {
	addr     string
	listener net.Listener
	handler  func(string) int
	mu       sync.Mutex
	commands []string
}

func newMockSSHServer(t *testing.T, handler func(string) int) *mockSSHServer {
	private, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(private)
	if err != nil {
		t.Fatalf("failed to create signer: %v", err)
	}
	config := &ssh.ServerConfig{NoClientAuth: true}
	config.AddHostKey(signer)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	srv := &mockSSHServer{addr: listener.Addr().String(), listener: listener, handler: handler}
	go srv.serve(config)
	return srv
}

func (s *mockSSHServer) serve(config *ssh.ServerConfig) {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		go func(nConn net.Conn) {
			serverConn, chans, reqs, err := ssh.NewServerConn(nConn, config)
			if err != nil {
				return
			}
			go ssh.DiscardRequests(reqs)
			for newCh := range chans {
				if newCh.ChannelType() != "session" {
					newCh.Reject(ssh.UnknownChannelType, "unknown channel type")
					continue
				}
				ch, requests, err := newCh.Accept()
				if err != nil {
					continue
				}
				go s.handleChannel(ch, requests)
			}
			serverConn.Close()
		}(conn)
	}
}

func (s *mockSSHServer) handleChannel(ch ssh.Channel, in <-chan *ssh.Request) {
	defer ch.Close()
	for req := range in {
		if req.Type == "exec" {
			var payload struct {
				Command string `ssh:"command"`
			}
			ssh.Unmarshal(req.Payload, &payload)
			s.mu.Lock()
			s.commands = append(s.commands, payload.Command)
			s.mu.Unlock()
			status := s.handler(payload.Command)
			req.Reply(true, nil)
			ch.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{uint32(status)}))
			return
		}
	}
}

func (s *mockSSHServer) Close() {
	s.listener.Close()
}

func (s *mockSSHServer) Commands() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.commands...)
}

func TestValidateRemoteCommand(t *testing.T) {
	server := newMockSSHServer(t, func(cmd string) int {
		if strings.Contains(cmd, "command -v echo") {
			return 0
		}
		if strings.Contains(cmd, "command -v nonexistent") {
			return 1
		}
		return 0
	})
	defer server.Close()

	client, err := ssh.Dial("tcp", server.addr, &ssh.ClientConfig{User: "test", HostKeyCallback: ssh.InsecureIgnoreHostKey()})
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}
	defer client.Close()

	if err := ValidateRemoteCommand(client, "echo"); err != nil {
		t.Fatalf("expected success, got %v", err)
	}

	if err := ValidateRemoteCommand(client, "nonexistent"); err == nil {
		t.Fatalf("expected error for nonexistent command")
	}
}

func TestRunRemoteScript(t *testing.T) {
	server := newMockSSHServer(t, func(cmd string) int { return 0 })
	defer server.Close()

	client, err := ssh.Dial("tcp", server.addr, &ssh.ClientConfig{User: "test", HostKeyCallback: ssh.InsecureIgnoreHostKey()})
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}
	defer client.Close()

	core, observed := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	SetLogger(logger)
	defer SetLogger(nil)

	script := "echo hi"
	if err := RunRemoteScript(client, script); err != nil {
		t.Fatalf("RunRemoteScript error: %v", err)
	}

	cmds := server.Commands()
	if len(cmds) != 1 || cmds[0] != script {
		t.Fatalf("expected command %q, got %v", script, cmds)
	}

	logs := observed.All()
	if len(logs) != 1 {
		t.Fatalf("expected one log entry, got %d", len(logs))
	}
	if logs[0].Message != "Running remote script" {
		t.Fatalf("unexpected log message %q", logs[0].Message)
	}
	if logs[0].ContextMap()["script"] != script {
		t.Fatalf("expected script %q in log, got %v", script, logs[0].ContextMap()["script"])
	}
}
