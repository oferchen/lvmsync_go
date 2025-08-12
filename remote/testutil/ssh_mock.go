package testutil

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net"
	"os"
	"sync"
	"testing"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

type ExecHandler func(string, ssh.Channel) int

type MockSSHServer struct {
	Addr       string
	listener   net.Listener
	handler    ExecHandler
	mu         sync.Mutex
	commands   []string
	globalReqs []string
	connCount  int
	PublicKey  ssh.PublicKey
}

func NewMockSSHServer(t *testing.T, handler func(string) int) *MockSSHServer {
	return NewMockSSHServerWithChannel(t, func(cmd string, ch ssh.Channel) int {
		return handler(cmd)
	})
}

func NewMockSSHServerWithChannel(t *testing.T, handler ExecHandler) *MockSSHServer {
	t.Helper()
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
	srv := &MockSSHServer{Addr: listener.Addr().String(), listener: listener, handler: handler, PublicKey: signer.PublicKey()}
	go srv.serve(config)
	return srv
}

//nolint:revive // complexity acceptable for test server
func (s *MockSSHServer) serve(config *ssh.ServerConfig) {
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

func (s *MockSSHServer) handleChannel(ch ssh.Channel, in <-chan *ssh.Request) {
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
			req.Reply(true, nil) //nolint:errcheck
			status := s.handler(payload.Command, ch)
			var exitStatus uint32
			if status >= 0 && status <= 255 {
				exitStatus = uint32(status)
			} else {
				exitStatus = 255
			}
			exitPayload := struct{ Status uint32 }{Status: exitStatus}
			ch.SendRequest("exit-status", false, ssh.Marshal(exitPayload)) //nolint:errcheck
			return
		}
	}
}

func (s *MockSSHServer) handleRequests(in <-chan *ssh.Request) {
	for req := range in {
		s.mu.Lock()
		s.globalReqs = append(s.globalReqs, req.Type)
		s.mu.Unlock()
		req.Reply(true, nil) //nolint:errcheck
	}
}

func (s *MockSSHServer) Close() {
	s.listener.Close() //nolint:errcheck
}

func (s *MockSSHServer) Commands() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.commands...)
}

func (s *MockSSHServer) GlobalRequests() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.globalReqs...)
}

func (s *MockSSHServer) ConnectionCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.connCount
}

func CreateKnownHostsFile(t *testing.T, server *MockSSHServer) string {
	t.Helper()
	host, portStr, err := net.SplitHostPort(server.Addr)
	if err != nil {
		t.Fatalf("SplitHostPort: %v", err)
	}
	line := knownhosts.Line([]string{fmt.Sprintf("[%s]:%s", host, portStr)}, server.PublicKey)
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

func CreateEmptyKnownHosts(t *testing.T) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "known_hosts")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	return f.Name()
}

func CreateTempKey(t *testing.T) string {
	t.Helper()
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
