package ssh

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"lvmsync_go/transport"
)

func emptyKnownHosts(t *testing.T) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "known_hosts")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	f.Close()
	return f.Name()
}

func TestUnknownHostKeyFails(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	hostKeyPath := filepath.Join(dir, "host_key")
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	keyBytes := x509.MarshalPKCS1PrivateKey(key)
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: keyBytes})
	if err := os.WriteFile(hostKeyPath, pemBytes, 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	srvIface, err := New(ctx, transport.Config{Logger: zap.NewNop(), SSHUser: "u", SSHPassword: "p", HostKeyPath: hostKeyPath, AllowInsecure: true})
	if err != nil {
		t.Fatalf("new server transport: %v", err)
	}
	srv := srvIface.(*Transport)
	ln, err := srv.Listen(ctx, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	acceptErr := make(chan error, 1)
	go func() {
		_, err := ln.Accept()
		acceptErr <- err
	}()

	kh := emptyKnownHosts(t)
	cliIface, err := New(ctx, transport.Config{Logger: zap.NewNop(), SSHUser: "u", SSHPassword: "p", SSHKnownHosts: kh, HostKeyPath: hostKeyPath})
	if err != nil {
		t.Fatalf("new client transport: %v", err)
	}
	cli := cliIface.(*Transport)
	dialCtx, cancel := context.WithTimeout(ctx, time.Second)
	_, err = cli.Dial(dialCtx, ln.Addr().String())
	cancel()
	if err == nil || !strings.Contains(err.Error(), "key is unknown") {
		t.Fatalf("expected unknown host key error, got %v", err)
	}
	if err := <-acceptErr; err == nil {
		t.Fatalf("expected server accept error")
	}
}
