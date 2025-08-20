//go:build quic

package quic

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
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

func TestQUICTransportHandshake(t *testing.T) {
	cert, _ := generateSelfSignedCert(t)
	core, logs := observer.New(zap.InfoLevel)
	trIface, err := New(transport.Config{Logger: zap.New(core), ClientCert: cert, AllowInsecure: true})
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
		qconn := conn.(*Conn)
		srvCtx, cancel := context.WithTimeout(baseCtx, time.Second)
		peerHS, err := tr.Negotiate(srvCtx, qconn, transport.Server, hs)
		cancel()
		if err != nil {
			t.Errorf("server negotiate: %v", err)
			return
		}
		if peerHS.ResumeToken != hs.ResumeToken || peerHS.DedupMode != hs.DedupMode || peerHS.BlockSize != hs.BlockSize || peerHS.Compress != hs.Compress || peerHS.Digest != hs.Digest || peerHS.MaxInFlight != hs.MaxInFlight || peerHS.CDCMin != hs.CDCMin || peerHS.CDCAvg != hs.CDCAvg || peerHS.CDCMax != hs.CDCMax {
			t.Errorf("unexpected peer handshake: %+v", peerHS)
		}
		buf := make([]byte, 1)
		qconn.Read(buf)
		qconn.Close()
		close(done)
	}()
	dialCtx, cancel := context.WithTimeout(baseCtx, time.Second)
	conn, err := tr.Dial(dialCtx, ln.Addr().String())
	cancel()
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	qconn := conn.(*Conn)
	negCtx, cancel := context.WithTimeout(baseCtx, time.Second)
	peerHS, err := tr.Negotiate(negCtx, qconn, transport.Client, hs)
	cancel()
	if err != nil {
		t.Fatalf("client negotiate: %v", err)
	}
	if peerHS.ResumeToken != hs.ResumeToken || peerHS.DedupMode != hs.DedupMode || peerHS.BlockSize != hs.BlockSize || peerHS.Compress != hs.Compress || peerHS.Digest != hs.Digest || peerHS.MaxInFlight != hs.MaxInFlight || peerHS.CDCMin != hs.CDCMin || peerHS.CDCAvg != hs.CDCAvg || peerHS.CDCMax != hs.CDCMax {
		t.Fatalf("unexpected peer handshake: %+v", peerHS)
	}
	qconn.Write([]byte{1})
	qconn.Close()
	<-done

	checkLogFields(t, logs, "dial_start", 1, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "dial_end", 1, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "listen_start", 1, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "listen_end", 1, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "negotiate_start", 2, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "negotiate_end", 2, false, zapcore.InfoLevel)
	checkHandshakeFields(t, logs, "negotiate_end", 2)
}

func TestQUICTransportSelectBestHandshake(t *testing.T) {
	cert, _ := generateSelfSignedCert(t)
	trIface, err := New(transport.Config{Logger: zap.NewNop(), ClientCert: cert, AllowInsecure: true})
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
		qconn := conn.(*Conn)
		srvCtx, cancel := context.WithTimeout(baseCtx, time.Second)
		_, err = tr.Negotiate(srvCtx, qconn, transport.Server, srvHS)
		cancel()
		if err == nil {
			buf := make([]byte, 1)
			qconn.Read(buf)
		}
		qconn.Close()
		srvErr <- err
	}()
	<-srvErr
}

func TestQUICTransportHandshakeError(t *testing.T) {
	cert, _ := generateSelfSignedCert(t)
	core, logs := observer.New(zap.InfoLevel)
	trIface, err := New(transport.Config{Logger: zap.New(core), ClientCert: cert, AllowInsecure: true})
	if err != nil {
		t.Fatalf("new transport: %v", err)
	}
	tr := trIface.(*Transport)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
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
		qconn := conn.(*Conn)
		qconn.Write([]byte("bad\n"))
		qconn.Close()
		close(done)
	}()

	conn, err := tr.Dial(ctx, ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	qconn := conn.(*Conn)
	if _, err := tr.Negotiate(ctx, qconn, transport.Client, common.Handshake{CDCMin: 64, CDCAvg: 128, CDCMax: 256, CRC32C: true}); err == nil {
		t.Fatalf("expected negotiate error")
	}
	qconn.Close()
	<-done

	checkLogFields(t, logs, "dial_start", 1, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "dial_end", 1, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "listen_start", 1, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "listen_end", 1, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "negotiate_start", 1, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "negotiate_end", 1, true, zapcore.ErrorLevel)
}

func TestQUICTransportHandshakeCDCMismatch(t *testing.T) {
	cert, _ := generateSelfSignedCert(t)
	core, logs := observer.New(zap.InfoLevel)
	trIface, err := New(transport.Config{Logger: zap.New(core), ClientCert: cert, AllowInsecure: true})
	if err != nil {
		t.Fatalf("new transport: %v", err)
	}
	tr := trIface.(*Transport)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
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
		qconn := conn.(*Conn)
		if _, err := tr.Negotiate(ctx, qconn, transport.Server, common.Handshake{ResumeToken: "tok", MaxInFlight: 8, CDCMin: 64, CDCAvg: 128, CDCMax: 256, CRC32C: true}); err == nil {
			t.Errorf("expected server negotiate error")
		}
		qconn.Close()
		close(done)
	}()

	conn, err := tr.Dial(ctx, ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	qconn := conn.(*Conn)
	if _, err := tr.Negotiate(ctx, qconn, transport.Client, common.Handshake{ResumeToken: "tok", MaxInFlight: 8, CDCMin: 64, CDCAvg: 256, CDCMax: 256, CRC32C: true}); err == nil {
		t.Fatalf("expected client negotiate error")
	}
	qconn.Close()
	<-done

	checkLogFields(t, logs, "dial_start", 1, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "dial_end", 1, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "listen_start", 1, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "listen_end", 1, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "negotiate_start", 2, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "negotiate_end", 2, true, zapcore.ErrorLevel)
}

func TestQUICTransportAllowInsecureWarn(t *testing.T) {
	core, obs := observer.New(zap.WarnLevel)
	if _, err := New(transport.Config{Logger: zap.New(core), AllowInsecure: true}); err != nil {
		t.Fatalf("New: %v", err)
	}
	entries := obs.FilterMessage("allow_insecure_enabled").All()
	if len(entries) != 1 {
		t.Fatalf("expected 1 warning log, got %d", len(entries))
	}
	if tr := entries[0].ContextMap()["transport"]; tr != "quic" {
		t.Fatalf("unexpected transport %v", tr)
	}
}

func TestQUICTransportRequiresLogger(t *testing.T) {
	if _, err := New(transport.Config{AllowInsecure: true}); err == nil {
		t.Fatalf("expected error when logger is nil")
	}
}

func TestQUICTransportRequiresRootsOrAllowInsecure(t *testing.T) {
	if _, err := New(transport.Config{Logger: zap.NewNop()}); err == nil {
		t.Fatalf("expected error when roots are nil without AllowInsecure")
	}
	if _, err := New(transport.Config{Logger: zap.NewNop(), AllowInsecure: true}); err != nil {
		t.Fatalf("allow insecure should permit missing roots: %v", err)
	}
}

func TestQUICTransportRequiresClientCert(t *testing.T) {
	root := x509.NewCertPool()
	if _, err := New(transport.Config{Logger: zap.NewNop(), Roots: root}); err == nil {
		t.Fatalf("expected error when client cert is nil")
	}
	if _, err := New(transport.Config{Logger: zap.NewNop(), Roots: root, AllowInsecure: true}); err != nil {
		t.Fatalf("allow insecure should permit missing client cert: %v", err)
	}
}

func TestConnDatagramReadDeadline(t *testing.T) {
	cert, _ := generateSelfSignedCert(t)
	trIface, err := New(transport.Config{Logger: zap.NewNop(), ClientCert: cert, AllowInsecure: true})
	if err != nil {
		t.Fatalf("new transport: %v", err)
	}
	tr := trIface.(*Transport)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ln, err := tr.Listen(ctx, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	ready := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			done <- fmt.Errorf("accept: %w", err)
			return
		}
		qconn := conn.(*Conn)
		defer qconn.Close()
		qconn.SetReadDeadline(time.Now().Add(-time.Second))
		if _, err := qconn.ReceiveDatagram(ctx); !errors.Is(err, context.DeadlineExceeded) {
			done <- fmt.Errorf("expected deadline exceeded, got %v", err)
			return
		}
		qconn.SetReadDeadline(time.Time{})
		close(ready)
		b, err := qconn.ReceiveDatagram(ctx)
		if err != nil {
			done <- fmt.Errorf("receive datagram: %w", err)
			return
		}
		if string(b) != "hi" {
			done <- fmt.Errorf("unexpected datagram %q", b)
			return
		}
		done <- nil
	}()

	conn, err := tr.Dial(ctx, ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	qconn := conn.(*Conn)
	defer qconn.Close()

	<-ready
	if err := qconn.SendDatagram([]byte("hi")); err != nil {
		t.Fatalf("send datagram: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestConnDatagramContextDeadline(t *testing.T) {
	cert, _ := generateSelfSignedCert(t)
	trIface, err := New(transport.Config{Logger: zap.NewNop(), ClientCert: cert, AllowInsecure: true})
	if err != nil {
		t.Fatalf("new transport: %v", err)
	}
	tr := trIface.(*Transport)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ln, err := tr.Listen(ctx, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	done := make(chan error, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			done <- fmt.Errorf("accept: %w", err)
			return
		}
		qconn := conn.(*Conn)
		defer qconn.Close()
		qconn.SetReadDeadline(time.Now().Add(time.Second))
		cctx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
		defer cancel()
		if _, err := qconn.ReceiveDatagram(cctx); !errors.Is(err, context.DeadlineExceeded) {
			done <- fmt.Errorf("expected context deadline exceeded, got %v", err)
			return
		}
		done <- nil
	}()

	conn, err := tr.Dial(ctx, ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	conn.Close()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestConnStreamDeadlines(t *testing.T) {
	cert, _ := generateSelfSignedCert(t)
	trIface, err := New(transport.Config{Logger: zap.NewNop(), ClientCert: cert, AllowInsecure: true})
	if err != nil {
		t.Fatalf("new transport: %v", err)
	}
	tr := trIface.(*Transport)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ln, err := tr.Listen(ctx, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	ready := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			done <- fmt.Errorf("accept: %w", err)
			return
		}
		qconn := conn.(*Conn)
		defer qconn.Close()
		hs := common.Handshake{CDCMin: 64, CDCAvg: 128, CDCMax: 256, CRC32C: true}
		if _, err := tr.Negotiate(ctx, qconn, transport.Server, hs); err != nil {
			done <- fmt.Errorf("server negotiate: %w", err)
			return
		}
		if qconn.LocalAddr() == nil || qconn.RemoteAddr() == nil {
			done <- fmt.Errorf("missing addr")
			return
		}
		close(ready)
		qconn.SetDeadline(time.Now().Add(-time.Millisecond))
		buf := make([]byte, 1)
		if _, err := qconn.Read(buf); err == nil {
			done <- errors.New("expected read timeout")
			return
		} else if ne, ok := err.(net.Error); !ok || !ne.Timeout() {
			done <- fmt.Errorf("expected timeout error, got %v", err)
			return
		}
		qconn.SetDeadline(time.Time{})
		qconn.SetWriteDeadline(time.Now().Add(-time.Millisecond))
		if _, err := qconn.Write([]byte{1}); err == nil {
			done <- errors.New("expected write timeout")
			return
		} else if ne, ok := err.(net.Error); !ok || !ne.Timeout() {
			done <- fmt.Errorf("expected timeout error, got %v", err)
			return
		}
		done <- nil
	}()

	conn, err := tr.Dial(ctx, ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	qconn := conn.(*Conn)
	defer qconn.Close()
	hs := common.Handshake{CDCMin: 64, CDCAvg: 128, CDCMax: 256, CRC32C: true}
	if _, err := tr.Negotiate(ctx, qconn, transport.Client, hs); err != nil {
		t.Fatalf("client negotiate: %v", err)
	}
	<-ready
	if qconn.LocalAddr() == nil || qconn.RemoteAddr() == nil {
		t.Fatalf("missing addr")
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestQUICNegotiateContextCancel(t *testing.T) {
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
