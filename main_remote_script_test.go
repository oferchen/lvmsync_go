package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"

	"lvmsync_go/config"
)

// mockSSHServer is copied from remote package tests for use in integration tests.
type mockSSHServer struct {
	addr      string
	listener  net.Listener
	handler   func(string) int
	mu        sync.Mutex
	commands  []string
	publicKey ssh.PublicKey
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

//nolint:revive // complexity okay for test server
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

func (s *mockSSHServer) Close() {
	s.listener.Close() //nolint:errcheck
}

func (s *mockSSHServer) Commands() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.commands...)
}

func createKnownHostsFile(t *testing.T, server *mockSSHServer) string {
	t.Helper()
	host, portStr, err := net.SplitHostPort(server.addr)
	if err != nil {
		t.Fatalf("SplitHostPort: %v", err)
	}
	line := knownhosts.Line([]string{fmt.Sprintf("[%s]:%s", host, portStr)}, server.publicKey)
	f, err := os.CreateTemp(t.TempDir(), "known_hosts")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	if _, err := f.WriteString(line + "\n"); err != nil {
		t.Fatalf("WriteString: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	return f.Name()
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

// Test that remote post script executes even when dumpChanges fails
func TestRemotePostScriptRunsOnError(t *testing.T) {
	server := newMockSSHServer(t, func(cmd string) int { return 0 })
	defer server.Close()

	host, portStr, _ := strings.Cut(server.addr, ":")
	port, errConv := strconv.Atoi(portStr)
	if errConv != nil {
		t.Fatalf("Atoi: %v", errConv)
	}

	var err error
	cfg, err = config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig returned error: %v", err)
	}
	cfg.RemotePreScript = "pre-script"
	cfg.RemotePostScript = "post-script"
	cfg.SSHUser = "test"
	cfg.SSHPort = port
	cfg.SSHKeyPath = createTempKey(t)
	cfg.KnownHosts = createKnownHostsFile(t, server)
	cfg.StrictHostKeyCheck = true
	cfg.LVMSyncPath = "lvmsync"

	original := dumpChangesSequential
	dumpChangesSequential = func(c *config.Config, snapshot, source string, out io.Writer) error {
		return io.ErrUnexpectedEOF
	}
	defer func() { dumpChangesSequential = original }()

	dest := host + ":/dev/null"
	err = runClientMode("/dev/snap", dest)
	if err == nil || !strings.Contains(err.Error(), "dumpChanges") {
		t.Fatalf("expected dumpChanges error, got %v", err)
	}

	cmds := server.Commands()
	if len(cmds) != 4 {
		t.Fatalf("expected 4 commands, got %d: %v", len(cmds), cmds)
	}
	if cmds[0] != cfg.RemotePreScript || cmds[3] != cfg.RemotePostScript {
		t.Fatalf("unexpected command order: %v", cmds)
	}
}

// Test that post script is not executed when pre script fails
func TestRemotePostScriptNotRunIfPreScriptFails(t *testing.T) {
	server := newMockSSHServer(t, func(cmd string) int {
		if cmd == "fail-pre" {
			return 1
		}
		return 0
	})
	defer server.Close()

	host, portStr, _ := strings.Cut(server.addr, ":")
	port, errConv := strconv.Atoi(portStr)
	if errConv != nil {
		t.Fatalf("Atoi: %v", errConv)
	}

	var err error
	cfg, err = config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig returned error: %v", err)
	}
	cfg.RemotePreScript = "fail-pre"
	cfg.RemotePostScript = "post-script"
	cfg.SSHUser = "test"
	cfg.SSHPort = port
	cfg.SSHKeyPath = createTempKey(t)
	cfg.KnownHosts = createKnownHostsFile(t, server)
	cfg.StrictHostKeyCheck = true
	cfg.LVMSyncPath = "lvmsync"

	dest := host + ":/dev/null"
	err = runClientMode("/dev/snap", dest)
	if err == nil {
		t.Fatalf("expected error from pre-script")
	}

	cmds := server.Commands()
	if len(cmds) != 1 || cmds[0] != cfg.RemotePreScript {
		t.Fatalf("post script should not run, commands: %v", cmds)
	}
}
