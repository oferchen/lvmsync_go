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
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	"golang.org/x/crypto/ssh"

	"github.com/oferchen/lvmsync_go/transport"
)

func checkLog(t *testing.T, logs *observer.ObservedLogs, msg string, wantErr bool, level zapcore.Level) {
	entries := logs.FilterMessage(msg).All()
	if len(entries) != 1 {
		t.Fatalf("expected 1 %s log, got %d", msg, len(entries))
	}
	e := entries[0]
	if e.Level != level {
		t.Fatalf("expected level %v for %s log, got %v", level, msg, e.Level)
	}
	ctx := e.ContextMap()
	for _, k := range []string{"address", "role", "duration_ms"} {
		if _, ok := ctx[k]; !ok {
			t.Fatalf("expected field %q in %s log", k, msg)
		}
	}
	if _, ok := ctx["error"]; wantErr && !ok {
		t.Fatalf("expected error field in %s log", msg)
	} else if !wantErr && ok {
		t.Fatalf("unexpected error field in %s log", msg)
	}
}

func TestSSHHostKeyPinMismatch(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	hostKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate host key: %v", err)
	}
	hostBytes := x509.MarshalPKCS1PrivateKey(hostKey)
	hostPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: hostBytes})
	hostPath := filepath.Join(dir, "host_key")
	if err := os.WriteFile(hostPath, hostPEM, 0600); err != nil {
		t.Fatalf("write host key: %v", err)
	}
	srvIface, err := New(ctx, transport.Config{Logger: zap.NewNop(), SSHUser: "u", SSHPassword: "p", HostKeyPath: hostPath, AllowInsecure: true})
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

	wrongKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate wrong key: %v", err)
	}
	wrongSigner, err := ssh.NewSignerFromKey(wrongKey)
	if err != nil {
		t.Fatalf("new signer: %v", err)
	}
	wrongHostKey := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(wrongSigner.PublicKey())))

	core, logs := observer.New(zap.InfoLevel)
	cliIface, err := New(ctx, transport.Config{Logger: zap.New(core), SSHUser: "u", SSHPassword: "p", SSHHostKey: wrongHostKey, HostKeyPath: hostPath})
	if err != nil {
		t.Fatalf("new client transport: %v", err)
	}
	cli := cliIface.(*Transport)
	dialCtx, cancel := context.WithTimeout(ctx, time.Second)
	_, err = cli.Dial(dialCtx, ln.Addr().String())
	cancel()
	if err == nil {
		t.Fatalf("expected dial error")
	}
	if !strings.Contains(err.Error(), "host key") {
		t.Fatalf("expected host key error, got %v", err)
	}
	if err := <-acceptErr; err == nil {
		t.Fatalf("expected server accept error")
	}

	checkLog(t, logs, "dial_start", false, zapcore.InfoLevel)
	checkLog(t, logs, "dial_end", true, zapcore.ErrorLevel)
}
