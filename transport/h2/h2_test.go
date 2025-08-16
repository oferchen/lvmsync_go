package h2

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
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	"golang.org/x/net/http2"

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

func TestDialTLS(t *testing.T) {
	cert, pool := generateSelfSignedCert(t)
	serverConf := &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS13, MaxVersion: tls.VersionTLS13}
	ln, err := tls.Listen("tcp", "127.0.0.1:0", serverConf)
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
		io.ReadAll(conn)
	}()
	clientConf := &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS13, MaxVersion: tls.VersionTLS13}
	core, logs := observer.New(zapcore.InfoLevel)
	logger := zap.New(core)
	if conn, err := dialTLS(context.Background(), ln.Addr().String(), clientConf, logger); err != nil {
		t.Fatalf("dialTLS: %v", err)
	} else {
		conn.Close()
	}
	checkLogFields(t, logs, "tls_handshake_start", 1, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "tls_handshake_end", 1, false, zapcore.InfoLevel)

	badConf := &tls.Config{RootCAs: x509.NewCertPool(), MinVersion: tls.VersionTLS13, MaxVersion: tls.VersionTLS13}
	core2, logs2 := observer.New(zapcore.InfoLevel)
	logger2 := zap.New(core2)
	if _, err := dialTLS(context.Background(), ln.Addr().String(), badConf, logger2); err == nil {
		t.Fatalf("expected error")
	}
	checkLogFields(t, logs2, "tls_handshake_start", 1, false, zapcore.InfoLevel)
	checkLogFields(t, logs2, "tls_handshake_end", 1, true, zapcore.ErrorLevel)
}

func TestPerformH2Handshake(t *testing.T) {
	cert, pool := generateSelfSignedCert(t)
	serverConf := &tls.Config{Certificates: []tls.Certificate{cert}, NextProtos: []string{"h2"}, MinVersion: tls.VersionTLS13, MaxVersion: tls.VersionTLS13}
	ln, err := tls.Listen("tcp", "127.0.0.1:0", serverConf)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	// successful handshake
	done := make(chan struct{})
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		tlsConn := conn.(*tls.Conn)
		fr := http2.NewFramer(tlsConn, tlsConn)
		preface := make([]byte, len(http2.ClientPreface))
		io.ReadFull(tlsConn, preface)
		fr.ReadFrame()
		fr.WriteSettings()
		fr.WriteSettingsAck()
		fr.ReadFrame()
		<-done
		conn.Close()
	}()
	clientConf := &tls.Config{RootCAs: pool, NextProtos: []string{"h2"}, MinVersion: tls.VersionTLS13, MaxVersion: tls.VersionTLS13}
	core, logs := observer.New(zapcore.InfoLevel)
	logger := zap.New(core)
	conn, err := dialTLS(context.Background(), ln.Addr().String(), clientConf, logger)
	if err != nil {
		t.Fatalf("dialTLS: %v", err)
	}
	if _, err := performH2Handshake(context.Background(), conn, logger); err != nil {
		t.Fatalf("handshake: %v", err)
	}
	close(done)
	conn.Close()

	checkLogFields(t, logs, "h2_handshake_start", 1, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "h2_handshake_end", 1, false, zapcore.InfoLevel)

	// failing handshake: server closes connection immediately; dialTLS should fail
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		conn.Close()
	}()
	if _, err := dialTLS(context.Background(), ln.Addr().String(), clientConf, zap.NewNop()); err == nil {
		t.Fatalf("expected dialTLS error")
	}
}

func TestLogDialResult(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	logDialResult(context.Background(), logger, "addr", "client", time.Now(), nil)
	checkLogFields(t, logs, "dial_end", 1, false, zapcore.InfoLevel)

	core2, logs2 := observer.New(zap.InfoLevel)
	logger2 := zap.New(core2)
	logDialResult(context.Background(), logger2, "addr", "client", time.Now(), errors.New("boom"))
	checkLogFields(t, logs2, "dial_end", 1, true, zapcore.ErrorLevel)
}

func TestH2TransportTLSHandshake(t *testing.T) {
	cert, pool := generateSelfSignedCert(t)
	core, logs := observer.New(zap.InfoLevel)
	trIface, err := New(transport.Config{Logger: zap.New(core), Roots: pool, ClientCert: cert, ServerCert: cert})
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
		if peerHS.ResumeToken != hs.ResumeToken || peerHS.DedupMode != hs.DedupMode || peerHS.BlockSize != hs.BlockSize || peerHS.Compress != hs.Compress || peerHS.Digest != hs.Digest || peerHS.MaxInFlight != hs.MaxInFlight || peerHS.ALPN != "h2" || peerHS.TLSVersion != "1.3" || peerHS.CDCMin != hs.CDCMin || peerHS.CDCAvg != hs.CDCAvg || peerHS.CDCMax != hs.CDCMax {
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
	if peerHS.ResumeToken != hs.ResumeToken || peerHS.DedupMode != hs.DedupMode || peerHS.BlockSize != hs.BlockSize || peerHS.Compress != hs.Compress || peerHS.Digest != hs.Digest || peerHS.MaxInFlight != hs.MaxInFlight || peerHS.ALPN != "h2" || peerHS.TLSVersion != "1.3" || peerHS.CDCMin != hs.CDCMin || peerHS.CDCAvg != hs.CDCAvg || peerHS.CDCMax != hs.CDCMax {
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

func TestH2TransportSelectBestHandshake(t *testing.T) {
	cert, pool := generateSelfSignedCert(t)
	trIface, err := New(transport.Config{Logger: zap.NewNop(), Roots: pool, ClientCert: cert, ServerCert: cert})
	if err != nil {
		t.Fatalf("new transport: %v", err)
	}
	tr := trIface.(*Transport)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	ln, err := tr.Listen(ctx, "127.0.0.1:0")
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

	srvHS := common.Handshake{
		DedupMode:   expDedup,
		Compress:    expCompress,
		Compressors: srvCompress,
		Digests:     srvDigest,
		ResumeToken: "tok",
		ODirect:     true,
		MaxInFlight: 8,
		CDCMin:      64,
		CDCAvg:      128,
		CDCMax:      256,
	}
	cliHS := common.Handshake{
		DedupMode:   expDedup,
		Compress:    expCompress,
		Compressors: cliCompress,
		Digests:     cliDigest,
		ResumeToken: "tok",
		ODirect:     true,
		MaxInFlight: 8,
		CDCMin:      64,
		CDCAvg:      128,
		CDCMax:      256,
	}

	srvErr := make(chan error, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			srvErr <- err
			return
		}
		_, err = tr.Negotiate(ctx, conn, transport.Server, srvHS)
		conn.Close()
		srvErr <- err
	}()

	conn, err := tr.Dial(ctx, ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	peer, err := tr.Negotiate(ctx, conn, transport.Client, cliHS)
	conn.Close()
	if err != nil {
		t.Fatalf("client negotiate: %v", err)
	}
	if err := <-srvErr; err != nil {
		t.Fatalf("server negotiate: %v", err)
	}
	if peer.DedupMode != expDedup || peer.Compress != expCompress || peer.Digest != expDigest || peer.ResumeToken != "tok" || !peer.ODirect || peer.CDCMin != 64 || peer.CDCAvg != 128 || peer.CDCMax != 256 || peer.ALPN != "h2" || peer.TLSVersion != "1.3" {
		t.Fatalf("unexpected peer handshake: %+v", peer)
	}
}

func TestH2TransportTLSHandshakeError(t *testing.T) {
	serverCert, pool := generateSelfSignedCert(t)
	clientCert, _ := generateSelfSignedCert(t)
	core, logs := observer.New(zap.InfoLevel)
	serverTrIface, err := New(transport.Config{Logger: zap.New(core), Roots: pool, ClientCert: serverCert, ServerCert: serverCert})
	if err != nil {
		t.Fatalf("server transport: %v", err)
	}
	clientTrIface, err := New(transport.Config{Logger: zap.New(core), Roots: pool, ClientCert: clientCert, ServerCert: clientCert})
	if err != nil {
		t.Fatalf("client transport: %v", err)
	}
	serverTr := serverTrIface.(*Transport)
	clientTr := clientTrIface.(*Transport)
	ctx := context.Background()
	ln, err := serverTr.Listen(ctx, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	dctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	if _, err := clientTr.Dial(dctx, ln.Addr().String()); err == nil {
		t.Fatalf("expected dial error")
	}

	checkLogFields(t, logs, "dial_start", 1, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "dial_end", 1, true, zapcore.ErrorLevel)
	checkLogFields(t, logs, "listen_start", 1, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "listen_end", 1, false, zapcore.InfoLevel)
}

func TestH2TransportTLSCDCMismatch(t *testing.T) {
	cert, pool := generateSelfSignedCert(t)
	core, logs := observer.New(zap.InfoLevel)
	trIface, err := New(transport.Config{Logger: zap.New(core), Roots: pool, ClientCert: cert, ServerCert: cert})
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

	checkLogFields(t, logs, "dial_start", 1, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "dial_end", 1, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "listen_start", 1, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "listen_end", 1, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "negotiate_start", 2, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "negotiate_end", 2, true, zapcore.ErrorLevel)

	entries := logs.FilterMessage("negotiate_end").All()
	for _, e := range entries {
		if _, ok := e.ContextMap()["error"]; !ok {
			t.Fatalf("expected error field in negotiate_end log")
		}
	}
}

func TestH2DialUnreachable(t *testing.T) {
	cert, pool := generateSelfSignedCert(t)
	trIface, err := New(transport.Config{Logger: zap.NewNop(), Roots: pool, ClientCert: cert, ServerCert: cert})
	if err != nil {
		t.Fatalf("new transport: %v", err)
	}
	tr := trIface.(*Transport)
	ctx := context.Background()
	dctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	if _, err := tr.Dial(dctx, "203.0.113.1:1"); err == nil {
		t.Fatalf("expected error")
	} else {
		var netErr net.Error
		if !errors.Is(err, context.DeadlineExceeded) && !strings.Contains(err.Error(), "network is unreachable") && (!errors.As(err, &netErr) || !netErr.Timeout()) {
			t.Fatalf("unexpected error: %v", err)
		}
	}
}

func TestH2TransportRequiresLogger(t *testing.T) {
	if _, err := New(transport.Config{}); err == nil {
		t.Fatalf("expected error when logger is nil")
	}
}

func TestH2DialTimeout(t *testing.T) {
	cert, pool := generateSelfSignedCert(t)
	trIface, err := New(transport.Config{Logger: zap.NewNop(), Roots: pool, ClientCert: cert, ServerCert: cert})
	if err != nil {
		t.Fatalf("new transport: %v", err)
	}
	tr := trIface.(*Transport)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	tlsLn := tls.NewListener(ln, tr.serverConf)
	go func() {
		conn, err := tlsLn.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		time.Sleep(time.Second)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := tr.Dial(ctx, tlsLn.Addr().String()); err == nil {
		t.Fatalf("expected dial timeout")
	} else if ne, ok := err.(net.Error); !ok || !ne.Timeout() {
		t.Fatalf("expected timeout error, got %v", err)
	}
}

func TestH2AcceptTimeout(t *testing.T) {
	cert, pool := generateSelfSignedCert(t)
	trIface, err := New(transport.Config{Logger: zap.NewNop(), Roots: pool, ClientCert: cert, ServerCert: cert})
	if err != nil {
		t.Fatalf("new transport: %v", err)
	}
	tr := trIface.(*Transport)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	ln, err := tr.Listen(ctx, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	errCh := make(chan error, 1)
	go func() {
		_, err := ln.Accept()
		errCh <- err
	}()

	clientConf := &tls.Config{
		RootCAs:      pool,
		Certificates: []tls.Certificate{cert},
		NextProtos:   []string{"h2"},
		MinVersion:   tls.VersionTLS13,
		MaxVersion:   tls.VersionTLS13,
	}
	conn, err := tls.Dial("tcp", ln.Addr().String(), clientConf)
	if err != nil {
		t.Fatalf("tls dial: %v", err)
	}
	defer conn.Close()

	if err := <-errCh; err == nil {
		t.Fatalf("expected accept timeout")
	} else if ne, ok := err.(net.Error); !ok || !ne.Timeout() {
		t.Fatalf("expected timeout error, got %v", err)
	}
}

func TestH2NegotiateContextCancel(t *testing.T) {
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

func TestTLSVersionString(t *testing.T) {
	tests := []struct {
		version uint16
		want    string
	}{
		{tls.VersionTLS10, "1.0"},
		{tls.VersionTLS11, "1.1"},
		{tls.VersionTLS12, "1.2"},
		{tls.VersionTLS13, "1.3"},
		{0, "unknown"},
		{0xffff, "65535"},
	}
	for _, tt := range tests {
		if got := transport.TLSVersionString(tt.version); got != tt.want {
			t.Errorf("TLSVersionString(%#x) = %q, want %q", tt.version, got, tt.want)
		}
	}
}

func TestRoleString(t *testing.T) {
	tests := []struct {
		role transport.Role
		want string
	}{
		{transport.Client, "client"},
		{transport.Server, "server"},
		{transport.Role(99), ""},
	}
	for _, tt := range tests {
		if got := tt.role.String(); got != tt.want {
			t.Errorf("Role(%v).String() = %q, want %q", tt.role, got, tt.want)
		}
	}
}

type errConn struct {
	writeErr error
	closeErr error
}

func (e *errConn) Read(_ []byte) (int, error)  { return 0, io.EOF }
func (e *errConn) Write(_ []byte) (int, error) { return 0, e.writeErr }
func (e *errConn) Close() error                { return e.closeErr }
func (e *errConn) LocalAddr() net.Addr         { return nil }
func (e *errConn) RemoteAddr() net.Addr        { return nil }
func (e *errConn) SetDeadline(time.Time) error { return nil }
func (e *errConn) SetReadDeadline(time.Time) error {
	return nil
}
func (e *errConn) SetWriteDeadline(time.Time) error {
	return nil
}

func TestConnCloseWriteError(t *testing.T) {
	writeErr := errors.New("write fail")
	closeErr := errors.New("close fail")

	tests := []struct {
		name     string
		wc       *errConn
		wantErrs []error
	}{
		{"write_error", &errConn{writeErr: writeErr}, []error{writeErr}},
		{"write_and_close_error", &errConn{writeErr: writeErr, closeErr: closeErr}, []error{writeErr, closeErr}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Conn{Conn: tt.wc, fr: http2.NewFramer(tt.wc, tt.wc), streamID: 1}
			err := c.Close()
			if err == nil {
				t.Fatalf("expected error")
			}
			for _, we := range tt.wantErrs {
				if !errors.Is(err, we) {
					t.Fatalf("expected error %v, got %v", we, err)
				}
			}
		})
	}
}
