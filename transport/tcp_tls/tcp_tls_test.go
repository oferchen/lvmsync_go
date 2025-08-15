package tcp_tls

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"math/big"
	"net"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"lvmsync_go/common"
	"lvmsync_go/transport"
)

func checkLogFields(t *testing.T, logs *observer.ObservedLogs, msg string, expected int, wantErr bool, level zapcore.Level) {
	entries := logs.FilterMessage(msg).All()
	if len(entries) != expected {
		t.Fatalf("expected %d %s logs, got %d", expected, msg, len(entries))
	}
	for _, e := range entries {
		if e.Level != level {
			t.Fatalf("expected level %s for %s log, got %s", level, msg, e.Level)
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

func TestTCPTLSTransportHandshake(t *testing.T) {
	cert, root := generateSelfSignedCert(t)
	core, logs := observer.New(zap.InfoLevel)
	trIface, err := New(transport.Config{Logger: zap.New(core), Roots: root, ClientCert: cert})
	if err != nil {
		t.Fatalf("new transport: %v", err)
	}
	tr := trIface.(*Transport)
	ctx := context.Background()
	ln, err := tr.Listen(ctx, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	hs := common.Handshake{ResumeToken: "tok", DedupMode: "cdc", BlockSize: 4096, Compress: "zstd", Digest: "sha256", MaxInFlight: 8, CDCMin: 64, CDCAvg: 128, CDCMax: 256}
	done := make(chan struct{})
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			t.Errorf("accept: %v", err)
			return
		}
		peerHS, err := tr.Negotiate(ctx, conn, transport.Server, hs)
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

	conn, err := tr.Dial(ctx, ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	peerHS, err := tr.Negotiate(ctx, conn, transport.Client, hs)
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

func TestTCPTLSTransportHandshakeError(t *testing.T) {
	cert, root := generateSelfSignedCert(t)
	core, logs := observer.New(zap.InfoLevel)
	trIface, err := New(transport.Config{Logger: zap.New(core), Roots: root, ClientCert: cert})
	if err != nil {
		t.Fatalf("new transport: %v", err)
	}
	tr := trIface.(*Transport)
	ctx := context.Background()
	ln, err := tr.Listen(ctx, "127.0.0.1:0")
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
		conn.Write([]byte("bad\n"))
		conn.Close()
		close(done)
	}()

	conn, err := tr.Dial(ctx, ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	if _, err := tr.Negotiate(ctx, conn, transport.Client, common.Handshake{CDCMin: 64, CDCAvg: 128, CDCMax: 256}); err == nil {
		t.Fatalf("expected negotiate error")
	}
	conn.Close()
	<-done

	checkLogFields(t, logs, "dial_start", 1, true, zapcore.InfoLevel)
	checkLogFields(t, logs, "dial_end", 1, true, zapcore.InfoLevel)
	checkLogFields(t, logs, "listen_start", 1, true, zapcore.InfoLevel)
	checkLogFields(t, logs, "listen_end", 1, true, zapcore.InfoLevel)
	checkLogFields(t, logs, "negotiate_start", 1, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "negotiate_end", 1, true, zapcore.ErrorLevel)
}

func TestTCPTLSTransportCDCMismatch(t *testing.T) {
	cert, root := generateSelfSignedCert(t)
	core, logs := observer.New(zap.InfoLevel)
	trIface, err := New(transport.Config{Logger: zap.New(core), Roots: root, ClientCert: cert})
	if err != nil {
		t.Fatalf("new transport: %v", err)
	}
	tr := trIface.(*Transport)
	ctx := context.Background()
	ln, err := tr.Listen(ctx, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	done := make(chan struct{})
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			t.Errorf("accept: %v", err)
			close(done)
			return
		}
		if _, err := tr.Negotiate(ctx, conn, transport.Server, common.Handshake{ResumeToken: "tok", MaxInFlight: 8, CDCMin: 64, CDCAvg: 128, CDCMax: 256}); err == nil {
			t.Errorf("expected server negotiate error")
		}
		conn.Close()
		close(done)
	}()

	conn, err := tr.Dial(ctx, ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	if _, err := tr.Negotiate(ctx, conn, transport.Client, common.Handshake{ResumeToken: "tok", MaxInFlight: 8, CDCMin: 64, CDCAvg: 256, CDCMax: 256}); err == nil {
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

func TestTCPTLSDialErrorLogging(t *testing.T) {
	cert, root := generateSelfSignedCert(t)
	core, logs := observer.New(zap.InfoLevel)
	trIface, err := New(transport.Config{Logger: zap.New(core), Roots: root, ClientCert: cert})
	if err != nil {
		t.Fatalf("new transport: %v", err)
	}
	tr := trIface.(*Transport)
	ctx := context.Background()
	if _, err := tr.Dial(ctx, "127.0.0.1:65000"); err == nil {
		t.Fatalf("expected dial error")
	}
	checkLogFields(t, logs, "dial_start", 1, true, zapcore.InfoLevel)
	checkLogFields(t, logs, "dial_end", 1, true, zapcore.ErrorLevel)
}

func TestTCPTLSListenErrorLogging(t *testing.T) {
	cert, root := generateSelfSignedCert(t)
	core, logs := observer.New(zap.InfoLevel)
	trIface, err := New(transport.Config{Logger: zap.New(core), Roots: root, ClientCert: cert})
	if err != nil {
		t.Fatalf("new transport: %v", err)
	}
	tr := trIface.(*Transport)
	ctx := context.Background()
	if _, err := tr.Listen(ctx, "bad_address"); err == nil {
		t.Fatalf("expected listen error")
	}
	checkLogFields(t, logs, "listen_start", 1, true, zapcore.InfoLevel)
	checkLogFields(t, logs, "listen_end", 1, true, zapcore.ErrorLevel)
}

func TestTCPTLSNegotiateInvalidRole(t *testing.T) {
	cert, root := generateSelfSignedCert(t)
	trIface, err := New(transport.Config{Logger: zap.NewNop(), Roots: root, ClientCert: cert})
	if err != nil {
		t.Fatalf("new transport: %v", err)
	}
	tr := trIface.(*Transport)
	ctx := context.Background()
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()
	if _, err := tr.Negotiate(ctx, c1, transport.Role(99), common.Handshake{CDCMin: 64, CDCAvg: 128, CDCMax: 256}); err == nil {
		t.Fatalf("expected error for invalid role")
	}
}

func TestTCPTLSCertValidation(t *testing.T) {
	root := x509.NewCertPool()
	cert, _ := generateSelfSignedCert(t)
	trIface, _ := New(transport.Config{Roots: root, ClientCert: cert, Logger: zap.NewNop()})
	tr := trIface.(*Transport)
	ctx := context.Background()
	ln, err := tr.Listen(ctx, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	done := make(chan struct{})
	go func() {
		conn, _ := ln.Accept()
		if conn != nil {
			conn.Close()
		}
		close(done)
	}()
	if _, err := tr.Dial(ctx, ln.Addr().String()); err == nil {
		t.Fatalf("expected cert validation error")
	}
	<-done
}

func TestTCPTLSDialContextCancel(t *testing.T) {
	cert, root := generateSelfSignedCert(t)
	trIface, err := New(transport.Config{Logger: zap.NewNop(), Roots: root, ClientCert: cert})
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

func TestTCPTLSNegotiateContextCancel(t *testing.T) {
	tr := &Transport{logger: zap.NewNop()}
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	start := time.Now()
	if _, err := tr.Negotiate(ctx, c1, transport.Client, common.Handshake{CDCMin: 64, CDCAvg: 128, CDCMax: 256}); err == nil {
		t.Fatalf("expected negotiate error")
	} else {
		var ne net.Error
		if !errors.As(err, &ne) || !ne.Timeout() {
			t.Fatalf("expected timeout error, got %v", err)
		}
	}
	if time.Since(start) > 100*time.Millisecond {
		t.Fatalf("negotiation did not fail promptly")
	}
}

func TestTCPTLSTransportRequiresLogger(t *testing.T) {
	cert, root := generateSelfSignedCert(t)
	if _, err := New(transport.Config{Roots: root, ClientCert: cert}); err == nil {
		t.Fatalf("expected error when logger is nil")
	}
}

func TestTCPTLSTransportRequiresRootsOrAllowInsecure(t *testing.T) {
	if _, err := New(transport.Config{Logger: zap.NewNop()}); err == nil {
		t.Fatalf("expected error when roots are nil without AllowInsecure")
	}
	if _, err := New(transport.Config{Logger: zap.NewNop(), AllowInsecure: true}); err != nil {
		t.Fatalf("allow insecure should permit missing roots: %v", err)
	}
}

func TestTCPTLSTransportRequiresClientCert(t *testing.T) {
	root := x509.NewCertPool()
	if _, err := New(transport.Config{Logger: zap.NewNop(), Roots: root}); err == nil {
		t.Fatalf("expected error when client cert is nil")
	}
	if _, err := New(transport.Config{Logger: zap.NewNop(), Roots: root, AllowInsecure: true}); err != nil {
		t.Fatalf("allow insecure should permit missing client cert: %v", err)
	}
}

func generateSelfSignedCert(t *testing.T) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(1),
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour),
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	cert := tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
	parsed, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse cert: %v", err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(parsed)
	return cert, pool
}
