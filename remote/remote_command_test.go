package remote

import (
	"crypto/rand"
	"crypto/rsa"
	"net"
	"sync"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

type mockSSHServer struct {
	addr       string
	listener   net.Listener
	handler    func(string) int
	mu         sync.Mutex
	commands   []string
	globalReqs []string
	connCount  int
	publicKey  ssh.PublicKey
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
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	srv := &mockSSHServer{addr: listener.Addr().String(), listener: listener, handler: handler, publicKey: signer.PublicKey()}
	go srv.serve(config)
	return srv
}

//nolint:revive // complexity acceptable for test server
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
			s.mu.Lock()
			s.connCount++
			s.mu.Unlock()
			go s.handleRequests(reqs)
			for newCh := range chans {
				if newCh.ChannelType() != "session" {
					newCh.Reject(ssh.UnknownChannelType, "unknown channel type") //nolint:errcheck
					continue
				}
				ch, requests, err := newCh.Accept()
				if err != nil {
					continue
				}
				go s.handleChannel(ch, requests)
			}
			serverConn.Close() //nolint:errcheck
		}(conn)
	}
}

func (s *mockSSHServer) handleChannel(ch ssh.Channel, in <-chan *ssh.Request) {
	defer ch.Close() //nolint:errcheck
	for req := range in {
		if req.Type == "exec" {
			var payload struct {
				Command string `ssh:"command"`
			}
			if err := ssh.Unmarshal(req.Payload, &payload); err != nil {
				return
			}
			s.mu.Lock()
			s.commands = append(s.commands, payload.Command)
			s.mu.Unlock()
			status := s.handler(payload.Command)
			req.Reply(true, nil) //nolint:errcheck
			exitPayload := struct{ Status uint32 }{uint32(status)}
			ch.SendRequest("exit-status", false, ssh.Marshal(exitPayload)) //nolint:errcheck
			return
		}
	}
}

func (s *mockSSHServer) handleRequests(in <-chan *ssh.Request) {
	for req := range in {
		s.mu.Lock()
		s.globalReqs = append(s.globalReqs, req.Type)
		s.mu.Unlock()
		req.Reply(true, nil) //nolint:errcheck
	}
}

func (s *mockSSHServer) Close() {
	s.listener.Close() //nolint:errcheck
}

func (s *mockSSHServer) Commands() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.commands...)
}

func (s *mockSSHServer) GlobalRequests() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.globalReqs...)
}

func (s *mockSSHServer) ConnectionCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.connCount
}

func TestValidateRemoteCommand(t *testing.T) {
	server := newMockSSHServer(t, func(cmd string) int {
		switch cmd {
		case "echo --version":
			return 0
		case "nonexistent --version":
			return 127
		default:
			return 0
		}
	})
	defer server.Close()
	knownHosts := createKnownHostsFile(t, server)
	hostKeyCallback, hostKeyErr := knownhosts.New(knownHosts)
	if hostKeyErr != nil {
		t.Fatalf("knownhosts.New: %v", hostKeyErr)
	}
	client, dialErr := ssh.Dial("tcp", server.addr, &ssh.ClientConfig{User: "test", HostKeyCallback: hostKeyCallback})
	if dialErr != nil {
		t.Fatalf("failed to dial: %v", dialErr)
	}
	defer client.Close() //nolint:errcheck

	if validateErr := ValidateRemoteCommand(client, "echo"); validateErr != nil {
		t.Fatalf("expected success, got %v", validateErr)
	}

	if validateErr := ValidateRemoteCommand(client, "nonexistent"); validateErr == nil {
		t.Fatalf("expected error for nonexistent command")
	}
}

func TestRunRemoteScript(t *testing.T) {
	server := newMockSSHServer(t, func(cmd string) int { return 0 })
	defer server.Close()
	knownHosts := createKnownHostsFile(t, server)
	hostKeyCallback, hostKeyErr := knownhosts.New(knownHosts)
	if hostKeyErr != nil {
		t.Fatalf("knownhosts.New: %v", hostKeyErr)
	}
	client, dialErr := ssh.Dial("tcp", server.addr, &ssh.ClientConfig{User: "test", HostKeyCallback: hostKeyCallback})
	if dialErr != nil {
		t.Fatalf("failed to dial: %v", dialErr)
	}
	defer client.Close() //nolint:errcheck

	core, observed := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	SetLogger(logger)
	defer SetLogger(nil)

	script := "echo hi"
	if scriptErr := RunRemoteScript(client, script); scriptErr != nil {
		t.Fatalf("RunRemoteScript error: %v", scriptErr)
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
