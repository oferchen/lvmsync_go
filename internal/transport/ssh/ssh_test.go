package ssh

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"

	"go.uber.org/zap"

	"lvmsync_go/config"
	"lvmsync_go/internal/transport"
	remotetest "lvmsync_go/remote/testutil"
)

type mockServer struct {
	addr       string
	listener   net.Listener
	publicKey  ssh.PublicKey
	received   bytes.Buffer
	sendData   []byte
	sinkExit   int
	sourceExit int
}

func newMockServer(t *testing.T, sendData []byte) *mockServer {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		t.Fatalf("NewSignerFromKey: %v", err)
	}
	cfg := &ssh.ServerConfig{NoClientAuth: true}
	cfg.AddHostKey(signer)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	srv := &mockServer{addr: ln.Addr().String(), listener: ln, publicKey: signer.PublicKey(), sendData: sendData}
	go srv.serve(cfg)
	return srv
}

func (s *mockServer) serve(cfg *ssh.ServerConfig) {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		go func(c net.Conn) {
			serverConn, chans, reqs, err := ssh.NewServerConn(c, cfg)
			if err != nil {
				return
			}
			go func(in <-chan *ssh.Request) {
				for req := range in {
					req.Reply(true, nil) //nolint:errcheck
				}
			}(reqs)
			for ch := range chans {
				if ch.ChannelType() != "session" {
					ch.Reject(ssh.UnknownChannelType, "unsupported") //nolint:errcheck
					continue
				}
				channel, reqs, err := ch.Accept()
				if err != nil {
					continue
				}
				go s.handleChannel(channel, reqs)
			}
			serverConn.Close() //nolint:errcheck
		}(conn)
	}
}

func (s *mockServer) handleChannel(ch ssh.Channel, in <-chan *ssh.Request) {
	defer ch.Close() //nolint:errcheck
	for req := range in {
		if req.Type == "exec" {
			var payload struct {
				Command string `ssh:"command"`
			}
			if err := ssh.Unmarshal(req.Payload, &payload); err != nil {
				return
			}
			req.Reply(true, nil) //nolint:errcheck
			var status int
			switch payload.Command {
			case "sink":
				_, _ = io.Copy(&s.received, ch)
				status = s.sinkExit
			case "source":
				if len(s.sendData) > 0 {
					_, _ = ch.Write(s.sendData)
				}
				status = s.sourceExit
			default:
				status = 0
			}
			ch.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{uint32(status)})) //nolint:errcheck
			return
		}
	}
}

func (s *mockServer) Close() { s.listener.Close() }

func (s *mockServer) knownHostsFile(t *testing.T) string {
	t.Helper()
	_, portStr, err := net.SplitHostPort(s.addr)
	if err != nil {
		t.Fatalf("SplitHostPort: %v", err)
	}
	line := knownhosts.Line([]string{fmt.Sprintf("[localhost]:%s", portStr)}, s.publicKey)
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

func setupTransport(t *testing.T, srv *mockServer) (transport.Sender, transport.Receiver) {
	t.Helper()
	_, portStr, err := net.SplitHostPort(srv.addr)
	if err != nil {
		t.Fatalf("SplitHostPort: %v", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("Atoi: %v", err)
	}
	cfg := &config.Config{
		SSHUser:              "test",
		SSHKeyPath:           remotetest.CreateTempKey(t),
		SSHPort:              port,
		SSHTimeout:           time.Second,
		SSHKeepAliveInterval: time.Second,
		KnownHosts:           srv.knownHostsFile(t),
	}
	s, r, err := New(cfg, zap.NewNop())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s, r
}

func TestSSHSendReceive(t *testing.T) {
	payload := []byte("hello ssh")
	srv := newMockServer(t, payload)
	defer srv.Close()
	s, r := setupTransport(t, srv)
	sendData := []byte("payload")
	if err := s.Send(context.Background(), bytes.NewReader(sendData)); err != nil {
		t.Fatalf("send: %v", err)
	}
	if got := srv.received.Bytes(); !bytes.Equal(got, sendData) {
		t.Fatalf("server received %q want %q", got, sendData)
	}
	var buf bytes.Buffer
	if err := r.Receive(context.Background(), &buf); err != nil {
		t.Fatalf("receive: %v", err)
	}
	if !bytes.Equal(buf.Bytes(), payload) {
		t.Fatalf("got %q want %q", buf.Bytes(), payload)
	}
}

func TestSSHRegistered(t *testing.T) {
	if _, ok := transport.Get("ssh"); !ok {
		t.Fatalf("ssh transport not registered")
	}
}

func TestSSHContextCancel(t *testing.T) {
	srv := newMockServer(t, nil)
	defer srv.Close()
	s, r := setupTransport(t, srv)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.Send(ctx, bytes.NewReader(nil)); !errors.Is(err, context.Canceled) {
		t.Fatalf("send expected context.Canceled, got %v", err)
	}
	if err := r.Receive(ctx, io.Discard); !errors.Is(err, context.Canceled) {
		t.Fatalf("receive expected context.Canceled, got %v", err)
	}
}

func TestSSHErrors(t *testing.T) {
	srv := newMockServer(t, nil)
	srv.sinkExit = 1
	srv.sourceExit = 1
	defer srv.Close()
	s, r := setupTransport(t, srv)
	if err := s.Send(context.Background(), bytes.NewReader([]byte("x"))); err == nil {
		t.Fatalf("expected send error")
	}
	if err := r.Receive(context.Background(), io.Discard); err == nil {
		t.Fatalf("expected receive error")
	}
}
