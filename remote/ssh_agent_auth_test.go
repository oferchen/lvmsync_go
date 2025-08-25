package remote

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"net"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// startAgentWithKey starts an SSH agent with a single RSA key and returns the
// socket path, the public key, and a cleanup function.
func startAgentWithKey(t *testing.T) (string, ssh.PublicKey, func()) {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatalf("signer: %v", err)
	}
	dir := t.TempDir()
	sock := filepath.Join(dir, "agent.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	keyring := agent.NewKeyring()
	if err := keyring.Add(agent.AddedKey{PrivateKey: priv}); err != nil {
		t.Fatalf("add key: %v", err)
	}
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		agent.ServeAgent(keyring, conn)
	}()
	cleanup := func() { ln.Close() }
	return sock, signer.PublicKey(), cleanup
}

// startAuthServer starts a minimal SSH server that accepts the provided public
// key for authentication.
func startAuthServer(t *testing.T, allowed ssh.PublicKey) (string, func()) {
	t.Helper()
	hostKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("host key: %v", err)
	}
	hostSigner, err := ssh.NewSignerFromKey(hostKey)
	if err != nil {
		t.Fatalf("host signer: %v", err)
	}
	config := &ssh.ServerConfig{PublicKeyCallback: func(conn ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
		if bytes.Equal(key.Marshal(), allowed.Marshal()) {
			return nil, nil
		}
		return nil, fmt.Errorf("unauthorized key")
	}}
	config.AddHostKey(hostSigner)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				_, _, _, _ = ssh.NewServerConn(c, config)
			}(conn)
		}
	}()
	return ln.Addr().String(), func() { ln.Close() }
}

func TestNewSSHClientAgentAuthSuccess(t *testing.T) {
	sock, pubKey, stopAgent := startAgentWithKey(t)
	defer stopAgent()
	t.Setenv("SSH_AUTH_SOCK", sock)

	addr, stopServer := startAuthServer(t, pubKey)
	defer stopServer()
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("SplitHostPort: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("Atoi: %v", err)
	}
	client, err := NewSSHClient(ctx, host, "test", "", port, "", false, time.Second, time.Second, 0, zap.NewNop())
	if err != nil {
		t.Fatalf("NewSSHClient: %v", err)
	}
	if err := client.Close(); err != nil {
		t.Fatalf("client close: %v", err)
	}
}
