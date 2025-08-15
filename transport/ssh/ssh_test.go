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
	cfg := transport.Config{Logger: zap.New(core), SSHUser: "u", SSHPassword: "p", SSHKnownHosts: kh}
	if _, err := New(cfg); err != nil {
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
	cfg := transport.Config{Logger: zap.New(core), SSHUser: "u", SSHPassword: "p", SSHHostKey: hostKey}
	if _, err := New(cfg); err != nil {
		t.Fatalf("New: %v", err)
	}
}

func TestNewAllowInsecure(t *testing.T) {
	core, _ := observer.New(zap.InfoLevel)
	cfg := transport.Config{Logger: zap.New(core), SSHUser: "u", SSHPassword: "p", AllowInsecure: true}
	if _, err := New(cfg); err != nil {
		t.Fatalf("New: %v", err)
	}
}

func TestSSHTransportAuthSuccess(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	srvCfg := transport.Config{Logger: zap.New(core), SSHUser: "test", SSHPassword: "pass", AllowInsecure: true}
	serverIface, err := New(srvCfg)
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
	kh := writeKnownHosts(t, ln.Addr().String(), server.hostSigner.PublicKey())
	clientIface, err := New(transport.Config{Logger: zap.New(core), SSHUser: "test", SSHPassword: "pass", SSHKnownHosts: kh})
	if err != nil {
		t.Fatalf("new client transport: %v", err)
	}
	client := clientIface.(*Transport)

	hs := common.Handshake{ResumeToken: "tok", DedupMode: "cdc", BlockSize: 4096, Compress: "zstd", Digest: "sha256", MaxInFlight: 8, CDCMin: 64, CDCAvg: 128, CDCMax: 256}
	done := make(chan struct{})
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			t.Errorf("accept: %v", err)
			return
		}
		peerHS, err := server.Negotiate(ctx, conn, transport.Server, hs)
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

	conn, err := client.Dial(ctx, ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	peerHS, err := client.Negotiate(ctx, conn, transport.Client, hs)
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

	checkLogFields(t, logs, "dial_start", 1, true, zapcore.InfoLevel)
	checkLogFields(t, logs, "dial_end", 1, true, zapcore.InfoLevel)
	checkLogFields(t, logs, "listen_start", 1, true, zapcore.InfoLevel)
	checkLogFields(t, logs, "listen_end", 1, true, zapcore.InfoLevel)
	checkLogFields(t, logs, "negotiate_start", 2, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "negotiate_end", 2, false, zapcore.InfoLevel)
	checkHandshakeFields(t, logs, "negotiate_end", 2)
}

func TestSSHTransportKeyAuth(t *testing.T) {
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
	srvCfg := transport.Config{Logger: zap.New(core), SSHUser: "test", SSHKeyPath: keyPath, AllowInsecure: true}
	serverIface, err := New(srvCfg)
	if err != nil {
		t.Fatalf("new server transport: %v", err)
	}
	clientCfg := transport.Config{Logger: zap.New(core), SSHUser: "test", SSHKeyPath: keyPath, AllowInsecure: true}
	clientIface, err := New(clientCfg)
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
		peerHS, err := server.Negotiate(ctx, conn, transport.Server, common.Handshake{ResumeToken: "tok", MaxInFlight: 8, CDCMin: 64, CDCAvg: 128, CDCMax: 256})
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
	peerHS, err := client.Negotiate(ctx, conn, transport.Client, common.Handshake{ResumeToken: "tok", MaxInFlight: 8, CDCMin: 64, CDCAvg: 128, CDCMax: 256})
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

	checkLogFields(t, logs, "dial_start", 1, true, zapcore.InfoLevel)
	checkLogFields(t, logs, "dial_end", 1, true, zapcore.InfoLevel)
	checkLogFields(t, logs, "listen_start", 1, true, zapcore.InfoLevel)
	checkLogFields(t, logs, "listen_end", 1, true, zapcore.InfoLevel)
	checkLogFields(t, logs, "negotiate_start", 2, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "negotiate_end", 2, false, zapcore.InfoLevel)
}

func TestSSHTransportSelectBestHandshake(t *testing.T) {
	cfg := transport.Config{Logger: zap.NewNop(), SSHUser: "test", SSHPassword: "pass", AllowInsecure: true}
	serverIface, err := New(cfg)
	if err != nil {
		t.Fatalf("new server transport: %v", err)
	}
	clientIface, err := New(cfg)
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

	srvCompress := []string{"zstd", "lz4"}
	cliCompress := []string{"lz4"}
	srvDigest := []string{"sha256", "blake3"}
	cliDigest := []string{"blake3"}
	srvDedup := []string{"fixed", "cdc"}
	cliDedup := []string{"cdc"}
	expCompress := common.SelectBest(srvCompress, cliCompress)
	expDigest := common.SelectBest(srvDigest, cliDigest)
	expDedup := common.SelectBest(srvDedup, cliDedup)

	hs := common.Handshake{
		DedupMode:   expDedup,
		Compress:    expCompress,
		Digest:      expDigest,
		ResumeToken: "tok",
		ODirect:     true,
		MaxInFlight: 8,
		CDCMin:      64,
		CDCAvg:      128,
		CDCMax:      256,
	}

	srvCh := make(chan common.Handshake)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			t.Errorf("accept: %v", err)
			return
		}
		peer, err := server.Negotiate(ctx, conn, transport.Server, hs)
		if err != nil {
			t.Errorf("server negotiate: %v", err)
		}
		conn.Close()
		srvCh <- peer
	}()

	conn, err := client.Dial(ctx, ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	peer, err := client.Negotiate(ctx, conn, transport.Client, hs)
	conn.Close()
	if err != nil {
		t.Fatalf("client negotiate: %v", err)
	}
	srvPeer := <-srvCh
	for _, p := range []common.Handshake{peer, srvPeer} {
		if p.DedupMode != expDedup || p.Compress != expCompress || p.Digest != expDigest || p.ResumeToken != "tok" || !p.ODirect || p.CDCMin != 64 || p.CDCAvg != 128 || p.CDCMax != 256 {
			t.Fatalf("unexpected peer handshake: %+v", p)
		}
	}
}

func TestSSHTransportAgentFallback(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	t.Setenv("SSH_AUTH_SOCK", filepath.Join(os.TempDir(), "nonexistent"))
	cfg := transport.Config{Logger: zap.New(core), SSHUser: "test", SSHPassword: "pass", SSHUseAgent: true, AllowInsecure: true}
	serverIface, err := New(cfg)
	if err != nil {
		t.Fatalf("new server transport: %v", err)
	}
	clientIface, err := New(cfg)
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
		peerHS, err := server.Negotiate(ctx, conn, transport.Server, common.Handshake{ResumeToken: "tok", MaxInFlight: 8, CDCMin: 64, CDCAvg: 128, CDCMax: 256})
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
	peerHS, err := client.Negotiate(ctx, conn, transport.Client, common.Handshake{ResumeToken: "tok", MaxInFlight: 8, CDCMin: 64, CDCAvg: 128, CDCMax: 256})
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

	checkLogFields(t, logs, "dial_start", 1, true, zapcore.InfoLevel)
	checkLogFields(t, logs, "dial_end", 1, true, zapcore.InfoLevel)
	checkLogFields(t, logs, "listen_start", 1, true, zapcore.InfoLevel)
	checkLogFields(t, logs, "listen_end", 1, true, zapcore.InfoLevel)
	checkLogFields(t, logs, "negotiate_start", 2, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "negotiate_end", 2, false, zapcore.InfoLevel)
}

func TestSSHTransportAuthFailure(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	srvCfg := transport.Config{Logger: zap.New(core), SSHUser: "test", SSHPassword: "pass", AllowInsecure: true}
	serverIface, err := New(srvCfg)
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
	kh := writeKnownHosts(t, ln.Addr().String(), server.hostSigner.PublicKey())
	clientIface, err := New(transport.Config{Logger: zap.New(core), SSHUser: "test", SSHPassword: "wrong", SSHKnownHosts: kh})
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

	checkLogFields(t, logs, "dial_start", 1, true, zapcore.InfoLevel)
	checkLogFields(t, logs, "dial_end", 1, true, zapcore.ErrorLevel)
	checkLogFields(t, logs, "listen_start", 1, true, zapcore.InfoLevel)
	checkLogFields(t, logs, "listen_end", 1, true, zapcore.InfoLevel)
	checkLogFields(t, logs, "negotiate_start", 0, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "negotiate_end", 0, false, zapcore.InfoLevel)
}

func TestSSHTransportCDCMismatch(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	srvCfg := transport.Config{Logger: zap.New(core), SSHUser: "test", SSHPassword: "pass", AllowInsecure: true}
	serverIface, err := New(srvCfg)
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
	kh := writeKnownHosts(t, ln.Addr().String(), server.hostSigner.PublicKey())
	clientIface, err := New(transport.Config{Logger: zap.New(core), SSHUser: "test", SSHPassword: "pass", SSHKnownHosts: kh})
	if err != nil {
		t.Fatalf("new client transport: %v", err)
	}
	client := clientIface.(*Transport)

	done := make(chan struct{})
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			t.Errorf("accept: %v", err)
			close(done)
			return
		}
		if _, err := server.Negotiate(ctx, conn, transport.Server, common.Handshake{ResumeToken: "tok", MaxInFlight: 8, CDCMin: 64, CDCAvg: 128, CDCMax: 256}); err == nil {
			t.Errorf("expected server negotiate error")
		}
		conn.Close()
		close(done)
	}()

	conn, err := client.Dial(ctx, ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	if _, err := client.Negotiate(ctx, conn, transport.Client, common.Handshake{ResumeToken: "tok", MaxInFlight: 8, CDCMin: 64, CDCAvg: 256, CDCMax: 256}); err == nil {
		t.Fatalf("expected client negotiate error")
	}
	conn.Close()
	<-done

	checkLogFields(t, logs, "dial_start", 1, true, zapcore.InfoLevel)
	checkLogFields(t, logs, "dial_end", 1, true, zapcore.InfoLevel)
	checkLogFields(t, logs, "listen_start", 1, true, zapcore.InfoLevel)
	checkLogFields(t, logs, "listen_end", 1, true, zapcore.InfoLevel)
	checkLogFields(t, logs, "negotiate_start", 2, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "negotiate_end", 2, true, zapcore.ErrorLevel)
}

func TestSSHTransportNegotiateTimeoutClient(t *testing.T) {
	core, _ := observer.New(zap.InfoLevel)
	cfg := transport.Config{Logger: zap.New(core), SSHUser: "test", SSHPassword: "pass", AllowInsecure: true}
	serverIface, err := New(cfg)
	if err != nil {
		t.Fatalf("new server transport: %v", err)
	}
	clientIface, err := New(cfg)
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
	if _, err := client.Negotiate(timeoutCtx, conn, transport.Client, common.Handshake{}); err == nil {
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
	serverIface, err := New(cfg)
	if err != nil {
		t.Fatalf("new server transport: %v", err)
	}
	clientIface, err := New(cfg)
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
		_, err = server.Negotiate(timeoutCtx, conn, transport.Server, common.Handshake{})
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

func TestSSHTransportRequiresLogger(t *testing.T) {
	if _, err := New(transport.Config{SSHUser: "u", SSHPassword: "p"}); err == nil {
		t.Fatalf("expected error when logger is nil")
	}
}

func TestSSHTransportRejectsUnknownHost(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	srvCfg := transport.Config{Logger: zap.New(core), SSHUser: "test", SSHPassword: "pass", AllowInsecure: true}
	serverIface, err := New(srvCfg)
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
	clientIface, err := New(transport.Config{Logger: zap.New(core), SSHUser: "test", SSHPassword: "pass", SSHKnownHosts: kh})
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
	checkLogFields(t, logs, "dial_start", 1, true, zapcore.InfoLevel)
	checkLogFields(t, logs, "dial_end", 1, true, zapcore.ErrorLevel)
}
