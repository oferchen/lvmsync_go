package quic

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"go.uber.org/zap"

	"lvmsync_go/common"
	"lvmsync_go/transport"
)

// TestIntegrationQUIC verifies handshake, bidirectional streams, and datagrams.
func TestIntegrationQUIC(t *testing.T) {
	cert, roots := generateSelfSignedCert(t)
	trIface, err := New(transport.Config{Logger: zap.NewNop(), Roots: roots, ClientCert: cert, ServerCert: cert})
	if err != nil {
		t.Fatalf("new transport: %v", err)
	}
	tr := trIface.(*Transport)

	baseCtx := context.Background()
	listenCtx, cancelListen := context.WithTimeout(baseCtx, time.Second)
	ln, err := tr.Listen(listenCtx, "127.0.0.1:0")
	defer cancelListen()
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	hs := common.Handshake{CDCMin: 64, CDCAvg: 128, CDCMax: 256, CRC32C: true}
	srvDone := make(chan struct{})
	go func() {
		defer close(srvDone)
		conn, err := ln.Accept()
		if err != nil {
			t.Errorf("accept: %v", err)
			return
		}
		qconn := conn.(*Conn)
		ctx, cancel := context.WithTimeout(baseCtx, time.Second)
		defer cancel()
		if _, err := tr.Negotiate(ctx, qconn, transport.Server, hs); err != nil {
			t.Errorf("server negotiate: %v", err)
			qconn.Close()
			return
		}
		buf := make([]byte, 5)
		if _, err := io.ReadFull(qconn, buf); err != nil {
			t.Errorf("server read: %v", err)
			qconn.Close()
			return
		}
		if string(buf) != "hello" {
			t.Errorf("server got %q", buf)
		}
		if _, err := qconn.Write([]byte("world")); err != nil {
			t.Errorf("server write: %v", err)
			qconn.Close()
			return
		}
		dctx, cancelD := context.WithTimeout(baseCtx, time.Second)
		dg, err := qconn.ReceiveDatagram(dctx)
		cancelD()
		if err != nil {
			t.Errorf("server receive datagram: %v", err)
			qconn.Close()
			return
		}
		if string(dg) != "ping" {
			t.Errorf("server datagram %q", dg)
		}
		if err := qconn.SendDatagram([]byte("pong")); err != nil {
			t.Errorf("server send datagram: %v", err)
		}
		qconn.Close()
	}()

	dialCtx, cancelDial := context.WithTimeout(baseCtx, time.Second)
	conn, err := tr.Dial(dialCtx, ln.Addr().String())
	cancelDial()
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	qconn := conn.(*Conn)
	negCtx, cancelNeg := context.WithTimeout(baseCtx, time.Second)
	if _, err := tr.Negotiate(negCtx, qconn, transport.Client, hs); err != nil {
		cancelNeg()
		t.Fatalf("client negotiate: %v", err)
	}
	cancelNeg()

	if _, err := qconn.Write([]byte("hello")); err != nil {
		t.Fatalf("client write: %v", err)
	}
	buf := make([]byte, 5)
	if _, err := io.ReadFull(qconn, buf); err != nil {
		t.Fatalf("client read: %v", err)
	}
	if string(buf) != "world" {
		t.Fatalf("client got %q", buf)
	}
	if err := qconn.SendDatagram([]byte("ping")); err != nil {
		t.Fatalf("client send datagram: %v", err)
	}
	dctx, cancelD := context.WithTimeout(baseCtx, time.Second)
	dg, err := qconn.ReceiveDatagram(dctx)
	cancelD()
	if err != nil {
		t.Fatalf("client receive datagram: %v", err)
	}
	if string(dg) != "pong" {
		t.Fatalf("client datagram %q", dg)
	}
	qconn.Close()
	<-srvDone
}

// TestIntegrationClientServer verifies handshake and bidirectional data flow
// using separate client and server transports.
func TestIntegrationClientServer(t *testing.T) {
	cert, roots := generateSelfSignedCert(t)
	srvIface, err := New(transport.Config{Logger: zap.NewNop(), Roots: roots, ClientCert: cert, ServerCert: cert})
	if err != nil {
		t.Fatalf("new server transport: %v", err)
	}
	srv := srvIface.(*Transport)
	cliIface, err := New(transport.Config{Logger: zap.NewNop(), Roots: roots, ClientCert: cert, ServerCert: cert})
	if err != nil {
		t.Fatalf("new client transport: %v", err)
	}
	cli := cliIface.(*Transport)

	baseCtx := context.Background()
	listenCtx, cancelListen := context.WithTimeout(baseCtx, 5*time.Second)
	ln, err := srv.Listen(listenCtx, "127.0.0.1:0")
	defer cancelListen()
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	hs := common.Handshake{CDCMin: 64, CDCAvg: 128, CDCMax: 256, CRC32C: true}
	srvDone := make(chan struct{})
	ack := make(chan struct{})
	go func() {
		defer close(srvDone)
		conn, err := ln.Accept()
		if err != nil {
			t.Errorf("accept: %v", err)
			return
		}
		qconn := conn.(*Conn)
		ctx, cancel := context.WithTimeout(baseCtx, time.Second)
		defer cancel()
		if _, err := srv.Negotiate(ctx, qconn, transport.Server, hs); err != nil {
			t.Errorf("server negotiate: %v", err)
			qconn.Close()
			return
		}
		buf := make([]byte, 5)
		if _, err := io.ReadFull(qconn, buf); err != nil {
			t.Errorf("server read: %v", err)
			qconn.Close()
			return
		}
		if string(buf) != "hello" {
			t.Errorf("server got %q", buf)
		}
		if _, err := qconn.Write([]byte("world")); err != nil {
			t.Errorf("server write: %v", err)
			qconn.Close()
			return
		}
		<-ack
		qconn.Close()
	}()

	dialCtx, cancelDial := context.WithTimeout(baseCtx, 5*time.Second)
	conn, err := cli.Dial(dialCtx, ln.Addr().String())
	cancelDial()
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	qconn := conn.(*Conn)
	negCtx, cancelNeg := context.WithTimeout(baseCtx, time.Second)
	if _, err := cli.Negotiate(negCtx, qconn, transport.Client, hs); err != nil {
		cancelNeg()
		t.Fatalf("client negotiate: %v", err)
	}
	cancelNeg()
	if _, err := qconn.Write([]byte("hello")); err != nil {
		t.Fatalf("client write: %v", err)
	}
	buf := make([]byte, 5)
	if _, err := io.ReadFull(qconn, buf); err != nil {
		t.Fatalf("client read: %v", err)
	}
	if string(buf) != "world" {
		t.Fatalf("client got %q", buf)
	}
	close(ack)
	qconn.Close()
	<-srvDone
}

// TestIntegrationHandshakeTimeout verifies client-side handshake timeouts when
// the server does not respond.
func TestIntegrationHandshakeTimeout(t *testing.T) {
	cert, roots := generateSelfSignedCert(t)
	srvIface, err := New(transport.Config{Logger: zap.NewNop(), Roots: roots, ClientCert: cert, ServerCert: cert})
	if err != nil {
		t.Fatalf("new server transport: %v", err)
	}
	srv := srvIface.(*Transport)
	cliIface, err := New(transport.Config{Logger: zap.NewNop(), Roots: roots, ClientCert: cert, ServerCert: cert})
	if err != nil {
		t.Fatalf("new client transport: %v", err)
	}
	cli := cliIface.(*Transport)

	baseCtx := context.Background()
	listenCtx, cancelListen := context.WithTimeout(baseCtx, time.Second)
	ln, err := srv.Listen(listenCtx, "127.0.0.1:0")
	defer cancelListen()
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	srvDone := make(chan struct{})
	go func() {
		defer close(srvDone)
		conn, err := ln.Accept()
		if err != nil {
			t.Errorf("accept: %v", err)
			return
		}
		qconn := conn.(*Conn)
		time.Sleep(50 * time.Millisecond)
		qconn.Close()
	}()

	dialCtx, cancelDial := context.WithTimeout(baseCtx, time.Second)
	conn, err := cli.Dial(dialCtx, ln.Addr().String())
	cancelDial()
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	qconn := conn.(*Conn)
	ctx, cancel := context.WithTimeout(baseCtx, 10*time.Millisecond)
	defer cancel()
	if _, err := cli.Negotiate(ctx, qconn, transport.Client, common.Handshake{}); err == nil {
		t.Fatalf("expected negotiate timeout")
	} else {
		var ne net.Error
		if !errors.As(err, &ne) || !ne.Timeout() {
			t.Fatalf("expected timeout, got %v", err)
		}
	}
	qconn.Close()
	<-srvDone
}

// TestIntegrationCertificateMismatch ensures connections fail when client and
// server use certificates from different roots.
func TestIntegrationCertificateMismatch(t *testing.T) {
	srvCert, srvRoots := generateSelfSignedCert(t)
	cliCert, cliRoots := generateSelfSignedCert(t)

	srvIface, err := New(transport.Config{Logger: zap.NewNop(), Roots: srvRoots, ClientCert: srvCert, ServerCert: srvCert})
	if err != nil {
		t.Fatalf("new server transport: %v", err)
	}
	srv := srvIface.(*Transport)
	baseCtx := context.Background()
	listenCtx, cancelListen := context.WithTimeout(baseCtx, time.Second)
	ln, err := srv.Listen(listenCtx, "127.0.0.1:0")
	defer cancelListen()
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	srvDone := make(chan struct{})
	go func() {
		defer close(srvDone)
		if conn, err := ln.Accept(); err == nil {
			conn.Close()
			t.Error("expected accept failure with certificate mismatch")
		}
	}()

	cliIface, err := New(transport.Config{Logger: zap.NewNop(), Roots: cliRoots, ClientCert: cliCert, ServerCert: cliCert})
	if err != nil {
		t.Fatalf("new client transport: %v", err)
	}
	cli := cliIface.(*Transport)
	dialCtx, cancelDial := context.WithTimeout(baseCtx, time.Second)
	_, err = cli.Dial(dialCtx, ln.Addr().String())
	cancelDial()
	if err == nil {
		t.Fatalf("expected dial failure with certificate mismatch")
	}
	<-srvDone
}

func TestIntegrationMissingTLSRoots(t *testing.T) {
	cert, _ := generateSelfSignedCert(t)
	if _, err := New(transport.Config{Logger: zap.NewNop(), ClientCert: cert, ServerCert: cert}); err == nil {
		t.Fatalf("expected error when roots missing")
	}
}

func TestIntegrationMissingClientCert(t *testing.T) {
	cert, roots := generateSelfSignedCert(t)
	if _, err := New(transport.Config{Logger: zap.NewNop(), Roots: roots, ServerCert: cert}); err == nil {
		t.Fatalf("expected error when client cert missing")
	}
}

func TestIntegrationMissingServerCert(t *testing.T) {
	cert, roots := generateSelfSignedCert(t)
	if _, err := New(transport.Config{Logger: zap.NewNop(), Roots: roots, ClientCert: cert}); err == nil {
		t.Fatalf("expected error when server cert missing")
	}
}
