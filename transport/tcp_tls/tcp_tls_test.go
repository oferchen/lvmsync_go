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
	trIface, err := New(transport.Config{Logger: zap.New(core), Roots: root, ClientCert: cert, ServerCert: cert})
	if err != nil {
		t.Fatalf("new transport: %v", err)
	}
	tr := trIface.(*Transport)
	baseCtx := context.Background()
	listenCtx, cancel := context.WithTimeout(baseCtx, time.Second)
	ln, err := tr.Listen(listenCtx, "127.0.0.1:0")
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
		peerHS, err := tr.Negotiate(srvCtx, conn, transport.Server, hs)
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
	conn, err := tr.Dial(dialCtx, ln.Addr().String())
	cancel()
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	negCtx, cancel := context.WithTimeout(baseCtx, time.Second)
	peerHS, err := tr.Negotiate(negCtx, conn, transport.Client, hs)
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
	checkHandshakeFields(t, logs, "negotiate_end", 2)
}

func TestTCPTLSALPNMatch(t *testing.T) {
	cert, root := generateSelfSignedCert(t)
	trIface, err := New(transport.Config{Logger: zap.NewNop(), Roots: root, ClientCert: cert, ServerCert: cert})
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
		if err == nil {
			io.Copy(io.Discard, conn)
			conn.Close()
		}
		close(done)
	}()

	dialCtx, cancel := context.WithTimeout(ctx, time.Second)
	conn, err := tr.Dial(dialCtx, ln.Addr().String())
	cancel()
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	tlsConn, ok := conn.(*tls.Conn)
	if !ok {
		t.Fatalf("expected tls.Conn")
	}
	state := tlsConn.ConnectionState()
	if state.NegotiatedProtocol != alpn {
		t.Fatalf("expected negotiated protocol %q, got %q", alpn, state.NegotiatedProtocol)
	}
	conn.Close()
	<-done
}

func TestTCPTLSALPNMismatch(t *testing.T) {
	cert, root := generateSelfSignedCert(t)
	core, logs := observer.New(zap.InfoLevel)
	trIface, err := New(transport.Config{Logger: zap.New(core), Roots: root, ClientCert: cert, ServerCert: cert})
	if err != nil {
		t.Fatalf("new transport: %v", err)
	}
	tr := trIface.(*Transport)
	tr.serverConf.NextProtos = []string{"other"}
	ctx := context.Background()
	ln, err := tr.Listen(ctx, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	done := make(chan struct{})
	go func() {
		conn, err := ln.Accept()
		if err == nil {
			io.Copy(io.Discard, conn)
			conn.Close()
		}
		close(done)
	}()

	dialCtx, cancel := context.WithTimeout(ctx, time.Second)
	_, err = tr.Dial(dialCtx, ln.Addr().String())
	cancel()
	if err == nil {
		t.Fatalf("expected alpn mismatch error")
	}
	<-done
	checkLogFields(t, logs, "dial_start", 1, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "dial_end", 1, true, zapcore.ErrorLevel)
}

func TestTCPTLSTransportSelectBestHandshake(t *testing.T) {
	cert, root := generateSelfSignedCert(t)
	trIface, err := New(transport.Config{Logger: zap.NewNop(), Roots: root, ClientCert: cert, ServerCert: cert})
	if err != nil {
		t.Fatalf("new transport: %v", err)
	}
	tr := trIface.(*Transport)
	baseCtx := context.Background()
	listenCtx, cancel := context.WithTimeout(baseCtx, time.Second)
	ln, err := tr.Listen(listenCtx, "127.0.0.1:0")
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
		Compress:    expCompress,
		Digest:      expDigest,
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
		srvCtx, cancel := context.WithTimeout(baseCtx, 5*time.Second)
		peer, err := tr.Negotiate(srvCtx, conn, transport.Server, srvHS)
		cancel()
		srvErr <- err
		if err == nil {
			_ = peer
			var buf [1]byte
			conn.Read(buf[:])
		}
		conn.Close()
	}()
	dialCtx, cancel := context.WithTimeout(baseCtx, time.Second)
	conn, err := tr.Dial(dialCtx, ln.Addr().String())
	cancel()
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	negCtx, cancel := context.WithTimeout(baseCtx, 5*time.Second)
	peer, err := tr.Negotiate(negCtx, conn, transport.Client, cliHS)
	cancel()
	if err != nil {
		conn.Close()
		t.Fatalf("client negotiate: %v", err)
	}

	select {
	case err := <-srvErr:
		if err != nil {
			conn.Close()
			t.Fatalf("server negotiate: %v", err)
		}
	case <-time.After(time.Second):
		conn.Close()
		t.Fatalf("server negotiate timeout")
	}

	if _, err := conn.Write([]byte{1}); err != nil {
		conn.Close()
		t.Fatalf("write: %v", err)
	}
	conn.Close()
	if peer.DedupMode != expDedup || peer.Compress != expCompress || peer.Digest != expDigest || peer.ResumeToken != "tok" || !peer.ODirect || peer.CDCMin != 64 || peer.CDCAvg != 128 || peer.CDCMax != 256 {
		t.Fatalf("unexpected peer handshake: %+v", peer)
	}
}

func TestTCPTLSTransportHandshakeError(t *testing.T) {
	cert, root := generateSelfSignedCert(t)
	core, logs := observer.New(zap.InfoLevel)
	trIface, err := New(transport.Config{Logger: zap.New(core), Roots: root, ClientCert: cert, ServerCert: cert})
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
	if _, err := tr.Negotiate(ctx, conn, transport.Client, common.Handshake{CDCMin: 64, CDCAvg: 128, CDCMax: 256, CRC32C: true}); err == nil {
		t.Fatalf("expected negotiate error")
	}
	conn.Close()
	<-done

	checkLogFields(t, logs, "dial_start", 1, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "dial_end", 1, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "listen_start", 1, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "listen_end", 1, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "negotiate_start", 1, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "negotiate_end", 1, true, zapcore.ErrorLevel)
}

func TestTCPTLSTransportCDCMismatch(t *testing.T) {
	cert, root := generateSelfSignedCert(t)
	core, logs := observer.New(zap.InfoLevel)
	trIface, err := New(transport.Config{Logger: zap.New(core), Roots: root, ClientCert: cert, ServerCert: cert})
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
		if _, err := tr.Negotiate(ctx, conn, transport.Server, common.Handshake{ResumeToken: "tok", MaxInFlight: 8, CDCMin: 64, CDCAvg: 128, CDCMax: 256, CRC32C: true}); err == nil {
			t.Errorf("expected server negotiate error")
		}
		conn.Close()
		close(done)
	}()

	conn, err := tr.Dial(ctx, ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	if _, err := tr.Negotiate(ctx, conn, transport.Client, common.Handshake{ResumeToken: "tok", MaxInFlight: 8, CDCMin: 64, CDCAvg: 256, CDCMax: 256, CRC32C: true}); err == nil {
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

func TestTCPTLSDialUnreachable(t *testing.T) {
	cert, root := generateSelfSignedCert(t)
	trIface, err := New(transport.Config{Logger: zap.NewNop(), Roots: root, ClientCert: cert, ServerCert: cert})
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
		time.Sleep(2 * time.Second)
	}()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err = tr.Dial(ctx, ln.Addr().String())
	if err == nil {
		t.Fatalf("expected timeout error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		var netErr net.Error
		if !errors.As(err, &netErr) || !netErr.Timeout() {
			t.Fatalf("expected context deadline exceeded or timeout, got %v", err)
		}
	}
}

func TestTCPTLSDialErrorLogging(t *testing.T) {
	cert, root := generateSelfSignedCert(t)
	core, logs := observer.New(zap.InfoLevel)
	trIface, err := New(transport.Config{Logger: zap.New(core), Roots: root, ClientCert: cert, ServerCert: cert})
	if err != nil {
		t.Fatalf("new transport: %v", err)
	}
	tr := trIface.(*Transport)
	ctx := context.Background()
	if _, err := tr.Dial(ctx, "127.0.0.1:65000"); err == nil {
		t.Fatalf("expected dial error")
	}
	checkLogFields(t, logs, "dial_start", 1, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "dial_end", 1, true, zapcore.ErrorLevel)
}

func TestTCPTLSListenErrorLogging(t *testing.T) {
	cert, root := generateSelfSignedCert(t)
	core, logs := observer.New(zap.InfoLevel)
	trIface, err := New(transport.Config{Logger: zap.New(core), Roots: root, ClientCert: cert, ServerCert: cert})
	if err != nil {
		t.Fatalf("new transport: %v", err)
	}
	tr := trIface.(*Transport)
	ctx := context.Background()
	if _, err := tr.Listen(ctx, "bad_address"); err == nil {
		t.Fatalf("expected listen error")
	}
	checkLogFields(t, logs, "listen_start", 1, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "listen_end", 1, true, zapcore.ErrorLevel)
}

func TestTCPTLSNegotiateInvalidRole(t *testing.T) {
	cert, root := generateSelfSignedCert(t)
	trIface, err := New(transport.Config{Logger: zap.NewNop(), Roots: root, ClientCert: cert, ServerCert: cert})
	if err != nil {
		t.Fatalf("new transport: %v", err)
	}
	tr := trIface.(*Transport)
	ctx := context.Background()
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()
	if _, err := tr.Negotiate(ctx, c1, transport.Role(99), common.Handshake{CDCMin: 64, CDCAvg: 128, CDCMax: 256, CRC32C: true}); err == nil {
		t.Fatalf("expected error for invalid role")
	}
}

func TestTCPTLSUnsupportedCipher(t *testing.T) {
	cert, root := generateSelfSignedCert(t)
	trIface, err := New(transport.Config{Logger: zap.NewNop(), Roots: root, ClientCert: cert, ServerCert: cert})
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
	srvErr := make(chan error, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			srvErr <- err
			return
		}
		defer conn.Close()
		tlsConn, ok := conn.(*tls.Conn)
		if !ok {
			srvErr <- errors.New("expected tls.Conn")
			return
		}
		hsCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		srvErr <- tlsConn.HandshakeContext(hsCtx)
		cancel()
	}()
	badCfg := &tls.Config{
		RootCAs:      root,
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
		MaxVersion:   tls.VersionTLS12,
		CipherSuites: []uint16{tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256},
		NextProtos:   []string{alpn},
	}
	if _, err := tls.Dial("tcp", ln.Addr().String(), badCfg); err == nil {
		t.Fatalf("expected handshake error")
	}
	select {
	case err := <-srvErr:
		if err == nil {
			t.Fatalf("expected server error")
		}
	case <-time.After(time.Second):
		t.Fatalf("server handshake timeout")
	}
}

func TestTCPTLSCertValidation(t *testing.T) {
	root := x509.NewCertPool()
	cert, _ := generateSelfSignedCert(t)
	trIface, _ := New(transport.Config{Roots: root, ClientCert: cert, ServerCert: cert, Logger: zap.NewNop()})
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
	trIface, err := New(transport.Config{Logger: zap.NewNop(), Roots: root, ClientCert: cert, ServerCert: cert})
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
	if _, err := tr.Negotiate(ctx, c1, transport.Client, common.Handshake{CDCMin: 64, CDCAvg: 128, CDCMax: 256, CRC32C: true}); err == nil {
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

func TestTCPTLSTransportAllowInsecureWarn(t *testing.T) {
	core, obs := observer.New(zap.WarnLevel)
	if _, err := New(transport.Config{Logger: zap.New(core), AllowInsecure: true}); err != nil {
		t.Fatalf("New: %v", err)
	}
	entries := obs.FilterMessage("allow_insecure_enabled").All()
	if len(entries) != 1 {
		t.Fatalf("expected 1 warning log, got %d", len(entries))
	}
	if tr := entries[0].ContextMap()["transport"]; tr != "tcp+tls" {
		t.Fatalf("unexpected transport %v", tr)
	}
}

func TestTCPTLSTransportLoggerOptional(t *testing.T) {
	cert, root := generateSelfSignedCert(t)
	if _, err := New(transport.Config{Roots: root, ClientCert: cert, ServerCert: cert}); err != nil {
		t.Fatalf("New: %v", err)
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

func TestTCPTLSTransportRequiresServerCert(t *testing.T) {
	root := x509.NewCertPool()
	cert, _ := generateSelfSignedCert(t)
	if _, err := New(transport.Config{Logger: zap.NewNop(), Roots: root, ClientCert: cert}); err == nil {
		t.Fatalf("expected error when server cert is nil")
	}
	if _, err := New(transport.Config{Logger: zap.NewNop(), Roots: root, ClientCert: cert, AllowInsecure: true}); err != nil {
		t.Fatalf("allow insecure should permit missing server cert: %v", err)
	}
}

func TestTCPTLSListenCloseErrorWarn(t *testing.T) {
	cert, root := generateSelfSignedCert(t)
	core, obs := observer.New(zap.WarnLevel)
	logger := zap.New(core)
	ctx, cancel := context.WithCancel(context.Background())
	trIface, err := New(transport.Config{Logger: logger, Roots: root, ClientCert: cert, ServerCert: cert})
	if err != nil {
		t.Fatalf("new transport: %v", err)
	}
	tr := trIface.(*Transport)
	ln, err := tr.Listen(ctx, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	if err := ln.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	cancel()
	time.Sleep(10 * time.Millisecond)
	entries := obs.FilterMessage("listener_close_error").All()
	if len(entries) != 1 {
		t.Fatalf("expected 1 warning log, got %d", len(entries))
	}
	if entries[0].Level != zapcore.WarnLevel {
		t.Fatalf("expected warn level, got %s", entries[0].Level)
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
