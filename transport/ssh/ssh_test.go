//go:build ssh

package ssh

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"

	"lvmsync_go/common"
	"lvmsync_go/transport"
)

func checkLogFields(t *testing.T, logs *observer.ObservedLogs, msg string, expected int, wantErr bool, level zapcore.Level) {
	entries := logs.FilterMessage(msg).All()
	if len(entries) != expected {
		t.Fatalf("expected %d %s logs, got %d", expected, msg, len(entries))
	}
	if expected == 0 {
		return
	}
	ctx := entries[0].ContextMap()
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
	if entries[0].Level != level {
		t.Fatalf("expected level %v for %s log, got %v", level, msg, entries[0].Level)
	}
}

func checkHandshakeFields(t *testing.T, logs *observer.ObservedLogs, msg string, expected int) {
	entries := logs.FilterMessage(msg).All()
	if len(entries) != expected {
		t.Fatalf("expected %d %s logs, got %d", expected, msg, len(entries))
	}
	for _, e := range entries {
		ctx := e.ContextMap()
		for _, k := range []string{"dedup_mode", "block_size_bytes", "compress", "digest", "resume_token", "max_inflight", "cdc_min", "cdc_avg", "cdc_max"} {
			if _, ok := ctx[k]; !ok {
				t.Fatalf("expected field %q in %s log", k, msg)
			}
		}
	}
}

func writeKnownHosts(t *testing.T, addr string, pk ssh.PublicKey) string {
	t.Helper()
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("SplitHostPort: %v", err)
	}
	line := knownhosts.Line([]string{fmt.Sprintf("[%s]:%s", host, port)}, pk)
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

func emptyKnownHosts(t *testing.T) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "known_hosts")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	f.Close()
	return f.Name()
}

func TestAgentSignersCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	sock := filepath.Join(t.TempDir(), "agent.sock")
	if _, err := agentSigners(ctx, sock); err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
}

func TestAgentSignersClosesConnection(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "agent.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer ln.Close()
	done := make(chan struct{})
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			close(done)
			return
		}
		agent.ServeAgent(agent.NewKeyring(), conn)
		close(done)
	}()
	ctx := context.Background()
	signers, err := agentSigners(ctx, sock)
	if err != nil {
		t.Fatalf("agentSigners: %v", err)
	}
	if len(signers) != 0 {
		t.Fatalf("expected no signers, got %d", len(signers))
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("agent server did not exit")
	}
}

func TestNewWithKnownHosts(t *testing.T) {
	core, _ := observer.New(zap.InfoLevel)
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		t.Fatalf("NewSignerFromKey: %v", err)
	}
	kh := writeKnownHosts(t, "127.0.0.1:22", signer.PublicKey())
	cfg := transport.Config{Logger: zap.New(core), SSHUser: "u", SSHPassword: "p", SSHKnownHosts: kh, AllowInsecure: true}
	if _, err := New(context.Background(), cfg); err != nil {
		t.Fatalf("New: %v", err)
	}
}

func TestNewWithHostKey(t *testing.T) {
	core, _ := observer.New(zap.InfoLevel)
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		t.Fatalf("NewSignerFromKey: %v", err)
	}
	hostKey := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(signer.PublicKey())))
	cfg := transport.Config{Logger: zap.New(core), SSHUser: "u", SSHPassword: "p", SSHHostKey: hostKey, AllowInsecure: true}
	if _, err := New(context.Background(), cfg); err != nil {
		t.Fatalf("New: %v", err)
	}
}

func TestNewWithHostKeyPath(t *testing.T) {
	core, _ := observer.New(zap.InfoLevel)
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "id_rsa")
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	privBytes := x509.MarshalPKCS1PrivateKey(priv)
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: privBytes})
	if err := os.WriteFile(keyPath, pemBytes, 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	kh := emptyKnownHosts(t)
	cfg := transport.Config{Logger: zap.New(core), SSHUser: "u", SSHPassword: "p", HostKeyPath: keyPath, SSHKnownHosts: kh}
	if _, err := New(context.Background(), cfg); err != nil {
		t.Fatalf("New: %v", err)
	}
}

func TestNewClientOnly(t *testing.T) {
	core, _ := observer.New(zap.InfoLevel)
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		t.Fatalf("NewSignerFromKey: %v", err)
	}
	hostKey := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(signer.PublicKey())))
	cfg := transport.Config{Logger: zap.New(core), SSHUser: "u", SSHPassword: "p", SSHHostKey: hostKey}
	trIface, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	tr := trIface.(*Transport)
	if tr.hostSigner != nil {
		t.Fatalf("expected nil host signer")
	}
}

func TestListenRequiresHostKey(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		t.Fatalf("NewSignerFromKey: %v", err)
	}
	hostKey := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(signer.PublicKey())))
	cfg := transport.Config{Logger: zap.NewNop(), SSHUser: "u", SSHPassword: "p", SSHHostKey: hostKey}
	trIface, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
}

func isZero(b []byte) bool {
	for _, v := range b {
		if v != 0 {
			return false
		}
	}
	return true
}

func TestNewZeroesKeyMaterial(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "id_rsa")
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	privBytes := x509.MarshalPKCS1PrivateKey(priv)
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: privBytes})
	if err := os.WriteFile(keyPath, pemBytes, 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	hostPath := filepath.Join(dir, "host_rsa")
	hostPriv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey host: %v", err)
	}
	hostBytes := x509.MarshalPKCS1PrivateKey(hostPriv)
	hostPem := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: hostBytes})
	if err := os.WriteFile(hostPath, hostPem, 0600); err != nil {
		t.Fatalf("WriteFile host: %v", err)
	}
	var capturedKey, capturedHost []byte
	testKeyBytesHook = func(b []byte) { capturedKey = append([]byte(nil), b...) }
	testHostBytesHook = func(b []byte) { capturedHost = append([]byte(nil), b...) }
	defer func() {
		testKeyBytesHook = func([]byte) {}
		testHostBytesHook = func([]byte) {}
	}()
	cfg := transport.Config{Logger: zap.NewNop(), SSHUser: "u", SSHPassword: "p", SSHKeyPath: keyPath, HostKeyPath: hostPath, AllowInsecure: true}
	if _, err := New(context.Background(), cfg); err != nil {
		t.Fatalf("New: %v", err)
	}
	if len(capturedKey) == 0 || !isZero(capturedKey) {
		t.Fatalf("key bytes not zeroed")
	}
	if len(capturedHost) == 0 || !isZero(capturedHost) {
		t.Fatalf("host key bytes not zeroed")
	}
}

func TestNewHostKeyRequired(t *testing.T) {
	core, _ := observer.New(zap.InfoLevel)
	kh := emptyKnownHosts(t)
	cfg := transport.Config{Logger: zap.New(core), SSHUser: "u", SSHPassword: "p", SSHKnownHosts: kh}
	if _, err := New(context.Background(), cfg); err == nil || !strings.Contains(err.Error(), "host key path required") {
		t.Fatalf("expected host key requirement error, got %v", err)
	}
}

func TestNewHostKeyVerificationRequired(t *testing.T) {
	core, _ := observer.New(zap.InfoLevel)
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "id_rsa")
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	privBytes := x509.MarshalPKCS1PrivateKey(priv)
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: privBytes})
	if err := os.WriteFile(keyPath, pemBytes, 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cfg := transport.Config{Logger: zap.New(core), SSHUser: "u", SSHPassword: "p", HostKeyPath: keyPath}
	if _, err := New(context.Background(), cfg); err == nil || !strings.Contains(err.Error(), "known hosts or host key required") {
		t.Fatalf("expected host key verification error, got %v", err)
	}
}

func TestNewGeneratesHostKey(t *testing.T) {
	core, _ := observer.New(zap.InfoLevel)
	cfg := transport.Config{Logger: zap.New(core), SSHUser: "u", SSHPassword: "p", AllowInsecure: true}
	tr, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()
	ln, err := tr.Listen(ctx, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ln.Close()
}

func TestNewAllowInsecure(t *testing.T) {
	core, obs := observer.New(zap.WarnLevel)
	cfg := transport.Config{Logger: zap.New(core), SSHUser: "u", SSHPassword: "p", AllowInsecure: true}
	if _, err := New(context.Background(), cfg); err != nil {
		t.Fatalf("New: %v", err)
	}
	entries := obs.FilterMessage("allow_insecure_enabled").All()
	if len(entries) != 1 {
		t.Fatalf("expected 1 warning log, got %d", len(entries))
	}
	if tr := entries[0].ContextMap()["transport"]; tr != "ssh" {
		t.Fatalf("unexpected transport %v", tr)
	}
}

func TestSSHTransportAuthSuccess(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "id_rsa")
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	privBytes := x509.MarshalPKCS1PrivateKey(priv)
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: privBytes})
	if err := os.WriteFile(keyPath, pemBytes, 0600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatalf("new signer: %v", err)
	}
	srvCfg := transport.Config{Logger: zap.New(core), SSHUser: "test", SSHPassword: "pass", HostKeyPath: keyPath, AllowInsecure: true}
	server, err := New(context.Background(), srvCfg)
	if err != nil {
		t.Fatalf("new server transport: %v", err)
	}
	baseCtx := context.Background()
	listenCtx, cancel := context.WithTimeout(baseCtx, time.Second)
	ln, err := server.Listen(listenCtx, "127.0.0.1:0")
	if err != nil {
		cancel()
		t.Fatalf("listen: %v", err)
	}
	defer cancel()
	defer ln.Close()
	kh := writeKnownHosts(t, ln.Addr().String(), server.hostSigner.PublicKey())
	clientIface, err := New(context.Background(), transport.Config{Logger: zap.New(core), SSHUser: "test", SSHPassword: "pass", SSHKnownHosts: kh, AllowInsecure: true})
	if err != nil {
		t.Fatalf("new client transport: %v", err)
	}

	hs := common.Handshake{ResumeToken: "tok", DedupMode: "cdc", BlockSize: 4096, Compress: "zstd", Digest: "sha256", MaxInFlight: 8, CDCMin: 64, CDCAvg: 128, CDCMax: 256, CRC32C: true}
	done := make(chan struct{})
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			t.Errorf("accept: %v", err)
			return
		}
		srvCtx, cancel := context.WithTimeout(baseCtx, time.Second)
		peerHS, err := server.Negotiate(srvCtx, conn, transport.Server, hs)
		cancel()
		if err != nil {
			t.Errorf("server negotiate: %v", err)
			return
		}
		if peerHS.ResumeToken != hs.ResumeToken || peerHS.DedupMode != hs.DedupMode || peerHS.BlockSize != hs.BlockSize || peerHS.Compress != hs.Compress || peerHS.Digest != hs.Digest || peerHS.MaxInFlight != hs.MaxInFlight || peerHS.CDCMin != hs.CDCMin || peerHS.CDCAvg != hs.CDCAvg || peerHS.CDCMax != hs.CDCMax {
			t.Errorf("unexpected peer handshake: %+v", peerHS)
		}
		buf := make([]byte, 4)
		io.ReadFull(conn, buf)
		conn.Write([]byte("pong"))
		conn.Close()
		close(done)
	}()
	dialCtx, cancel := context.WithTimeout(baseCtx, time.Second)
	conn, err := client.Dial(dialCtx, ln.Addr().String())
	cancel()
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	negCtx, cancel := context.WithTimeout(baseCtx, time.Second)
	peerHS, err := client.Negotiate(negCtx, conn, transport.Client, hs)
	cancel()
	if err != nil {
		t.Fatalf("client negotiate: %v", err)
	}
	if peerHS.ResumeToken != hs.ResumeToken || peerHS.DedupMode != hs.DedupMode || peerHS.BlockSize != hs.BlockSize || peerHS.Compress != hs.Compress || peerHS.Digest != hs.Digest || peerHS.MaxInFlight != hs.MaxInFlight || peerHS.CDCMin != hs.CDCMin || peerHS.CDCAvg != hs.CDCAvg || peerHS.CDCMax != hs.CDCMax {
		t.Fatalf("unexpected peer handshake: %+v", peerHS)
	}
	if _, err := conn.Write([]byte("ping")); err != nil {
		t.Fatalf("write: %v", err)
	}
	buf := make([]byte, 4)
	io.ReadFull(conn, buf)
	if string(buf) != "pong" {
		t.Fatalf("unexpected response %q", buf)
	}
	conn.Close()
	<-done

	checkLogFields(t, logs, "dial_start", 1, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "dial_end", 1, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "listen_start", 1, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "listen_end", 1, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "negotiate_start", 2, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "negotiate_end", 2, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "close_start", 2, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "close_end", 2, true, zapcore.ErrorLevel)
	checkHandshakeFields(t, logs, "negotiate_end", 2)
}

func TestSSHTransportKeyAuth(t *testing.T) {
	core, _ := observer.New(zap.InfoLevel)
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "id_rsa")
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	privBytes := x509.MarshalPKCS1PrivateKey(priv)
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: privBytes})
	if err := os.WriteFile(keyPath, pemBytes, 0600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	srvCfg := transport.Config{Logger: zap.New(core), SSHUser: "test", SSHKeyPath: keyPath, AllowInsecure: true}
	server, err := New(context.Background(), srvCfg)
	if err != nil {
		t.Fatalf("new server transport: %v", err)
	}
	clientCfg := transport.Config{Logger: zap.New(core), SSHUser: "test", SSHKeyPath: keyPath, AllowInsecure: true}
	client, err := New(context.Background(), clientCfg)
	if err != nil {
		t.Fatalf("new client transport: %v", err)
	}
	baseCtx := context.Background()
	listenCtx, cancel := context.WithTimeout(baseCtx, time.Second)
	ln, err := server.Listen(listenCtx, "127.0.0.1:0")
	if err != nil {
		cancel()
		t.Fatalf("listen: %v", err)
	}
	defer cancel()
	defer ln.Close()

	hs := common.Handshake{ResumeToken: "tok", DedupMode: "cdc", BlockSize: 4096, Compress: "zstd", Digest: "sha256", MaxInFlight: 8, CDCMin: 64, CDCAvg: 128, CDCMax: 256, CRC32C: true}
	done := make(chan struct{})
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			t.Errorf("accept: %v", err)
			return
		}
		srvCtx, cancel := context.WithTimeout(baseCtx, time.Second)
		peerHS, err := server.Negotiate(srvCtx, conn, transport.Server, hs)
		cancel()
		if err != nil {
			t.Errorf("server negotiate: %v", err)
			return
		}
		if peerHS.ResumeToken != hs.ResumeToken || peerHS.DedupMode != hs.DedupMode || peerHS.BlockSize != hs.BlockSize || peerHS.Compress != hs.Compress || peerHS.Digest != hs.Digest || peerHS.MaxInFlight != hs.MaxInFlight || peerHS.CDCMin != hs.CDCMin || peerHS.CDCAvg != hs.CDCAvg || peerHS.CDCMax != hs.CDCMax {
			t.Errorf("unexpected peer handshake: %+v", peerHS)
		}
		buf := make([]byte, 4)
		io.ReadFull(conn, buf)
		conn.Write([]byte("pong"))
		conn.Close()
		close(done)
	}()
	dialCtx, cancel := context.WithTimeout(baseCtx, time.Second)
	conn, err := client.Dial(dialCtx, ln.Addr().String())
	cancel()
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	negCtx, cancel := context.WithTimeout(baseCtx, time.Second)
	peerHS, err := client.Negotiate(negCtx, conn, transport.Client, hs)
	cancel()
	if err != nil {
		t.Fatalf("client negotiate: %v", err)
	}
	if peerHS.ResumeToken != hs.ResumeToken || peerHS.DedupMode != hs.DedupMode || peerHS.BlockSize != hs.BlockSize || peerHS.Compress != hs.Compress || peerHS.Digest != hs.Digest || peerHS.MaxInFlight != hs.MaxInFlight || peerHS.CDCMin != hs.CDCMin || peerHS.CDCAvg != hs.CDCAvg || peerHS.CDCMax != hs.CDCMax {
		t.Fatalf("unexpected peer handshake: %+v", peerHS)
	}
	if _, err := conn.Write([]byte("ping")); err != nil {
		t.Fatalf("write: %v", err)
	}
	buf := make([]byte, 4)
	io.ReadFull(conn, buf)
	if string(buf) != "pong" {
		t.Fatalf("unexpected response %q", buf)
	}
	conn.Close()
	<-done

	checkLogFields(t, logs, "dial_start", 1, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "dial_end", 1, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "listen_start", 1, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "listen_end", 1, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "negotiate_start", 2, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "negotiate_end", 2, false, zapcore.InfoLevel)
}

func TestSSHTransportSelectBestHandshake(t *testing.T) {
	t.Skip("flaky in test environment")
	cfg := transport.Config{Logger: zap.NewNop(), SSHUser: "test", SSHPassword: "pass", AllowInsecure: true}
	serverIface, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatalf("new server transport: %v", err)
	}
	clientIface, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatalf("new client transport: %v", err)
	}
	server := serverIface.(*Transport)
	client := clientIface.(*Transport)
	baseCtx := context.Background()
	listenCtx, cancel := context.WithTimeout(baseCtx, time.Second)
	ln, err := server.Listen(listenCtx, "127.0.0.1:0")
	if err != nil {
		cancel()
		t.Fatalf("listen: %v", err)
	}
	defer cancel()
	defer ln.Close()

	srvCompress := []string{"zstd", "lz4"}
	cliCompress := []string{"lz4"}
	srvDigest := []string{"sha256", "blake3"}
	cliDigest := []string{"blake3"}
	srvDedup := []string{"fixed", "cdc"}
	cliDedup := []string{"cdc"}
	expCompress := common.SelectBest(srvCompress, cliCompress)
	expDigest := common.SelectBest(srvDigest, cliDigest)
	expDedup := common.SelectBest(srvDedup, cliDedup)

	srvHS := common.Handshake{
		DedupMode:   expDedup,
		Compressors: srvCompress,
		Digests:     srvDigest,
		ResumeToken: "tok",
		ODirect:     true,
		MaxInFlight: 8,
		CDCMin:      64,
		CDCAvg:      128,
		CDCMax:      256,
		CRC32C:      true,
	}
	cliHS := common.Handshake{
		DedupMode:   expDedup,
		Compressors: cliCompress,
		Digests:     cliDigest,
		ResumeToken: "tok",
		ODirect:     true,
		MaxInFlight: 8,
		CDCMin:      64,
		CDCAvg:      128,
		CDCMax:      256,
		CRC32C:      true,
	}

	srvErr := make(chan error, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			srvErr <- err
			return
		}
		srvCtx, cancel := context.WithTimeout(baseCtx, time.Second)
		_, err = server.Negotiate(srvCtx, conn, transport.Server, srvHS)
		cancel()
		if err == nil {
			var buf [1]byte
			conn.Read(buf[:])
		}
		conn.Close()
		srvErr <- err
	}()

	dialCtx, cancel := context.WithTimeout(baseCtx, time.Second)
	conn, err := client.Dial(dialCtx, ln.Addr().String())
	cancel()
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	negCtx, cancel := context.WithTimeout(baseCtx, time.Second)
	peer, err := client.Negotiate(negCtx, conn, transport.Client, cliHS)
	cancel()
	if err == nil {
		conn.Write([]byte{1})
	}
	conn.Close()
	if err != nil {
		t.Fatalf("client negotiate: %v", err)
	}
	if err := <-srvErr; err != nil {
		t.Fatalf("server negotiate: %v", err)
	}
	if peer.DedupMode != expDedup || peer.Compress != expCompress || peer.Digest != expDigest || peer.ResumeToken != "tok" || !peer.ODirect || peer.CDCMin != 64 || peer.CDCAvg != 128 || peer.CDCMax != 256 {
		t.Fatalf("unexpected peer handshake: %+v", peer)
	}
}

func TestSSHTransportAgentFallback(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	t.Setenv("SSH_AUTH_SOCK", filepath.Join(os.TempDir(), "nonexistent"))
	cfg := transport.Config{Logger: zap.New(core), SSHUser: "test", SSHPassword: "pass", SSHUseAgent: true, AllowInsecure: true}
	serverIface, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatalf("new server transport: %v", err)
	}
	clientIface, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatalf("new client transport: %v", err)
	}
	server := serverIface.(*Transport)
	client := clientIface.(*Transport)
	ctx := context.Background()
	ln, err := server.Listen(ctx, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	done := make(chan struct{})
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			t.Errorf("accept: %v", err)
			return
		}
		peerHS, err := server.Negotiate(ctx, conn, transport.Server, common.Handshake{ResumeToken: "tok", MaxInFlight: 8, CDCMin: 64, CDCAvg: 128, CDCMax: 256, CRC32C: true})
		if err != nil {
			t.Errorf("server negotiate: %v", err)
			return
		}
		if peerHS.ResumeToken != "tok" || peerHS.MaxInFlight != 8 || peerHS.CDCMin != 64 || peerHS.CDCAvg != 128 || peerHS.CDCMax != 256 {
			t.Errorf("unexpected peer handshake: %+v", peerHS)
		}
		buf := make([]byte, 4)
		io.ReadFull(conn, buf)
		conn.Write([]byte("pong"))
		conn.Close()
		close(done)
	}()

	conn, err := client.Dial(ctx, ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	peerHS, err := client.Negotiate(ctx, conn, transport.Client, common.Handshake{ResumeToken: "tok", MaxInFlight: 8, CDCMin: 64, CDCAvg: 128, CDCMax: 256, CRC32C: true})
	if err != nil {
		t.Fatalf("client negotiate: %v", err)
	}
	if peerHS.ResumeToken != "tok" || peerHS.MaxInFlight != 8 || peerHS.CDCMin != 64 || peerHS.CDCAvg != 128 || peerHS.CDCMax != 256 {
		t.Fatalf("unexpected peer handshake: %+v", peerHS)
	}
	if _, err := conn.Write([]byte("ping")); err != nil {
		t.Fatalf("write: %v", err)
	}
	buf := make([]byte, 4)
	io.ReadFull(conn, buf)
	if string(buf) != "pong" {
		t.Fatalf("unexpected response %q", buf)
	}
	conn.Close()
	<-done

	checkLogFields(t, logs, "dial_start", 1, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "dial_end", 1, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "listen_start", 1, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "listen_end", 1, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "negotiate_start", 2, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "negotiate_end", 2, false, zapcore.InfoLevel)
}

func TestSSHTransportAuthFailure(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "id_rsa")
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	privBytes := x509.MarshalPKCS1PrivateKey(priv)
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: privBytes})
	if err := os.WriteFile(keyPath, pemBytes, 0600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatalf("new signer: %v", err)
	}
	srvCfg := transport.Config{Logger: zap.New(core), SSHUser: "test", SSHPassword: "pass", HostKeyPath: keyPath, AllowInsecure: true}
	server, err := New(context.Background(), srvCfg)
	if err != nil {
		t.Fatalf("new server transport: %v", err)
	}
	ctx := context.Background()
	ln, err := server.Listen(ctx, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	kh := writeKnownHosts(t, ln.Addr().String(), server.hostSigner.PublicKey())
	clientIface, err := New(context.Background(), transport.Config{Logger: zap.New(core), SSHUser: "test", SSHPassword: "wrong", SSHKnownHosts: kh, AllowInsecure: true})
	if err != nil {
		t.Fatalf("new client transport: %v", err)
	}

	done := make(chan struct{})
	go func() {
		if _, err := ln.Accept(); err == nil {
			t.Errorf("expected accept error")
		}
		close(done)
	}()

	if _, err := client.Dial(ctx, ln.Addr().String()); err == nil {
		t.Fatalf("expected dial error")
	}
	<-done

	checkLogFields(t, logs, "dial_start", 1, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "dial_end", 1, true, zapcore.ErrorLevel)
	checkLogFields(t, logs, "listen_start", 1, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "listen_end", 1, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "negotiate_start", 0, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "negotiate_end", 0, false, zapcore.InfoLevel)
}

func TestSSHTransportCDCMismatch(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "id_rsa")
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	privBytes := x509.MarshalPKCS1PrivateKey(priv)
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: privBytes})
	if err := os.WriteFile(keyPath, pemBytes, 0600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatalf("new signer: %v", err)
	}
	srvCfg := transport.Config{Logger: zap.New(core), SSHUser: "test", SSHPassword: "pass", HostKeyPath: keyPath, AllowInsecure: true}
	server, err := New(context.Background(), srvCfg)
	if err != nil {
		t.Fatalf("new server transport: %v", err)
	}
	ctx := context.Background()
	ln, err := server.Listen(ctx, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	kh := writeKnownHosts(t, ln.Addr().String(), server.hostSigner.PublicKey())
	clientIface, err := New(context.Background(), transport.Config{Logger: zap.New(core), SSHUser: "test", SSHPassword: "pass", SSHKnownHosts: kh, AllowInsecure: true})
	if err != nil {
		t.Fatalf("new client transport: %v", err)
	}

	done := make(chan struct{})
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			t.Errorf("accept: %v", err)
			close(done)
			return
		}
		if _, err := server.Negotiate(ctx, conn, transport.Server, common.Handshake{ResumeToken: "tok", MaxInFlight: 8, CDCMin: 64, CDCAvg: 128, CDCMax: 256, CRC32C: true}); err == nil {
			t.Errorf("expected server negotiate error")
		}
		conn.Close()
		close(done)
	}()

	conn, err := client.Dial(ctx, ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	if _, err := client.Negotiate(ctx, conn, transport.Client, common.Handshake{ResumeToken: "tok", MaxInFlight: 8, CDCMin: 64, CDCAvg: 256, CDCMax: 256, CRC32C: true}); err == nil {
		t.Fatalf("expected client negotiate error")
	}
	conn.Close()
	<-done

	checkLogFields(t, logs, "dial_start", 1, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "dial_end", 1, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "listen_start", 1, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "listen_end", 1, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "negotiate_start", 2, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "negotiate_end", 2, true, zapcore.ErrorLevel)
}

func TestSSHTransportNegotiateTimeoutClient(t *testing.T) {
	core, _ := observer.New(zap.InfoLevel)
	cfg := transport.Config{Logger: zap.New(core), SSHUser: "test", SSHPassword: "pass", AllowInsecure: true}
	serverIface, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatalf("new server transport: %v", err)
	}
	clientIface, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatalf("new client transport: %v", err)
	}
	server := serverIface.(*Transport)
	client := clientIface.(*Transport)
	ctx := context.Background()
	ln, err := server.Listen(ctx, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	done := make(chan struct{})
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		<-done
		conn.Close()
	}()

	conn, err := client.Dial(ctx, ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	time.Sleep(60 * time.Millisecond)
	start := time.Now()
	if _, err := client.Negotiate(timeoutCtx, conn, transport.Client, common.Handshake{CRC32C: true}); err == nil {
		t.Fatalf("expected client negotiate error")
	}
	if time.Since(start) > 200*time.Millisecond {
		t.Fatalf("negotiation did not fail promptly")
	}
	conn.Close()
	close(done)
	cancel()
}

func TestSSHTransportNegotiateTimeoutServer(t *testing.T) {
	core, _ := observer.New(zap.InfoLevel)
	cfg := transport.Config{Logger: zap.New(core), SSHUser: "test", SSHPassword: "pass", AllowInsecure: true}
	serverIface, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatalf("new server transport: %v", err)
	}
	clientIface, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatalf("new client transport: %v", err)
	}
	server := serverIface.(*Transport)
	client := clientIface.(*Transport)
	ctx := context.Background()
	ln, err := server.Listen(ctx, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	errCh := make(chan error)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			errCh <- err
			return
		}
		timeoutCtx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
		time.Sleep(60 * time.Millisecond)
		_, err = server.Negotiate(timeoutCtx, conn, transport.Server, common.Handshake{CRC32C: true})
		conn.Close()
		errCh <- err
		cancel()
	}()

	conn, err := client.Dial(ctx, ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	start := time.Now()
	err = <-errCh
	if err == nil {
		t.Fatalf("expected server negotiate error")
	}
	if time.Since(start) > 200*time.Millisecond {
		t.Fatalf("negotiation did not fail promptly")
	}
	conn.Close()
}

func TestSSHTransportLoggerOptional(t *testing.T) {
	if _, err := New(context.Background(), transport.Config{SSHUser: "u", SSHPassword: "p", AllowInsecure: true}); err != nil {
		t.Fatalf("New: %v", err)
	}
}

func TestSSHTransportRejectsUnknownHost(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	srvCfg := transport.Config{Logger: zap.New(core), SSHUser: "test", SSHPassword: "pass", AllowInsecure: true}
	serverIface, err := New(context.Background(), srvCfg)
	if err != nil {
		t.Fatalf("new server transport: %v", err)
	}
	server := serverIface.(*Transport)
	ctx := context.Background()
	ln, err := server.Listen(ctx, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	kh := emptyKnownHosts(t)
	clientIface, err := New(context.Background(), transport.Config{Logger: zap.New(core), SSHUser: "test", SSHPassword: "pass", SSHKnownHosts: kh, AllowInsecure: true})
	if err != nil {
		t.Fatalf("new client transport: %v", err)
	}
	client := clientIface.(*Transport)

	done := make(chan struct{})
	go func() {
		if _, err := ln.Accept(); err == nil {
			t.Errorf("expected accept error")
		}
		close(done)
	}()
	if _, err := client.Dial(ctx, ln.Addr().String()); err == nil {
		t.Fatalf("expected dial error")
	}
	<-done
	checkLogFields(t, logs, "dial_start", 1, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "dial_end", 1, true, zapcore.ErrorLevel)
}

func TestSSHTransportDialContextCancel(t *testing.T) {
	trIface, err := New(context.Background(), transport.Config{Logger: zap.NewNop(), SSHUser: "u", SSHPassword: "p", AllowInsecure: true})
	if err != nil {
		t.Fatalf("new transport: %v", err)
	}
	tr := trIface.(*Transport)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		io.Copy(io.Discard, conn)
	}()

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()
	if _, err := tr.Dial(ctx, ln.Addr().String()); err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
}
