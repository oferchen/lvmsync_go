package h2

import (
	"context"
	"io"
	"net"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/oferchen/lvmsync_go/common"
	"github.com/oferchen/lvmsync_go/transport"
)

// TestH2Integration spins up an in-process HTTP/2 listener and exercises a
// full dial/negotiate round trip through the transport registry.
func TestH2Integration(t *testing.T) {
	cert, pool := generateSelfSignedCert(t)
	core, logs := observer.New(zapcore.InfoLevel)
	trIface, err := transport.Get("h2", transport.Config{Logger: zap.New(core), Roots: pool, ClientCert: cert, ServerCert: cert})
	if err != nil {
		t.Fatalf("get transport: %v", err)
	}
	tr := trIface.(*Transport)
	cleared := false
	tr.clearDeadline = func(conn net.Conn) error {
		cleared = true
		return conn.SetDeadline(time.Time{})
	}

	baseCtx := context.Background()
	listenCtx, cancelListen := context.WithTimeout(baseCtx, time.Second)
	ln, err := tr.Listen(listenCtx, "127.0.0.1:0")
	cancelListen()
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	hs := common.Handshake{ResumeToken: "tok", DedupMode: "cdc", BlockSize: 4096, Compress: "zstd", Digest: "sha256", MaxInFlight: 8, CDCMin: 64, CDCAvg: 128, CDCMax: 256, CRC32C: true}

	srvDone := make(chan struct{})
	go func() {
		defer close(srvDone)
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
			conn.Close()
			return
		}
		if peerHS.ResumeToken != hs.ResumeToken || peerHS.DedupMode != hs.DedupMode || peerHS.BlockSize != hs.BlockSize || peerHS.Compress != hs.Compress || peerHS.Digest != hs.Digest || peerHS.MaxInFlight != hs.MaxInFlight || peerHS.ALPN != "h2" || peerHS.TLSVersion != "1.3" || peerHS.CDCMin != hs.CDCMin || peerHS.CDCAvg != hs.CDCAvg || peerHS.CDCMax != hs.CDCMax {
			t.Errorf("unexpected peer handshake: %+v", peerHS)
			conn.Close()
			return
		}
		buf := make([]byte, 5)
		if _, err := io.ReadFull(conn, buf); err != nil {
			t.Errorf("server read: %v", err)
			conn.Close()
			return
		}
		if string(buf) != "hello" {
			t.Errorf("server got %q", buf)
		}
		if _, err := conn.Write([]byte("world")); err != nil {
			t.Errorf("server write: %v", err)
		}
		conn.Close()
	}()

	dialCtx, cancelDial := context.WithTimeout(baseCtx, time.Second)
	conn, err := tr.Dial(dialCtx, ln.Addr().String())
	cancelDial()
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	negCtx, cancelNeg := context.WithTimeout(baseCtx, time.Second)
	peerHS, err := tr.Negotiate(negCtx, conn, transport.Client, hs)
	cancelNeg()
	if err != nil {
		t.Fatalf("client negotiate: %v", err)
	}
	if peerHS.ResumeToken != hs.ResumeToken || peerHS.DedupMode != hs.DedupMode || peerHS.BlockSize != hs.BlockSize || peerHS.Compress != hs.Compress || peerHS.Digest != hs.Digest || peerHS.MaxInFlight != hs.MaxInFlight || peerHS.ALPN != "h2" || peerHS.TLSVersion != "1.3" || peerHS.CDCMin != hs.CDCMin || peerHS.CDCAvg != hs.CDCAvg || peerHS.CDCMax != hs.CDCMax {
		t.Fatalf("unexpected peer handshake: %+v", peerHS)
	}
	if _, err := conn.Write([]byte("hello")); err != nil {
		t.Fatalf("client write: %v", err)
	}
	buf := make([]byte, 5)
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("client read: %v", err)
	}
	if string(buf) != "world" {
		t.Fatalf("client got %q", buf)
	}
	conn.Close()
	<-srvDone

	if !cleared {
		t.Fatalf("deadline not cleared")
	}
	checkLogFields(t, logs, "dial_start", 1, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "tls_handshake_start", 1, false, zapcore.InfoLevel)
        checkLogFields(t, logs, "tls_handshake_end", 1, false, zapcore.InfoLevel)
        checkTLSVersionField(t, logs, "tls_handshake_end", 1, "1.3")
        checkLogFields(t, logs, "h2_handshake_start", 1, false, zapcore.InfoLevel)
        checkLogFields(t, logs, "h2_handshake_end", 1, false, zapcore.InfoLevel)
        checkLogFields(t, logs, "dial_end", 1, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "listen_start", 1, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "listen_end", 1, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "negotiate_start", 2, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "negotiate_end", 2, false, zapcore.InfoLevel)
	checkHandshakeFields(t, logs, "negotiate_end", 2)
}
