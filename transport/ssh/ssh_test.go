package ssh

import (
	"context"
	"io"
	"testing"

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

func TestSSHTransportAuthSuccess(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	cfg := transport.Config{Logger: zap.New(core), SSHUser: "test", SSHPassword: "pass"}
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

	checkLogFields(t, logs, "dial_start", 1, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "dial_end", 1, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "listen_start", 1, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "listen_end", 1, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "negotiate_start", 2, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "negotiate_end", 2, false, zapcore.InfoLevel)
}

func TestSSHTransportAuthFailure(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	serverCfg := transport.Config{Logger: zap.New(core), SSHUser: "test", SSHPassword: "pass"}
	clientCfg := transport.Config{Logger: zap.New(core), SSHUser: "test", SSHPassword: "wrong"}
	serverIface, err := New(serverCfg)
	if err != nil {
		t.Fatalf("new server transport: %v", err)
	}
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
	cfg := transport.Config{Logger: zap.New(core), SSHUser: "test", SSHPassword: "pass"}
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

	checkLogFields(t, logs, "dial_start", 1, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "dial_end", 1, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "listen_start", 1, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "listen_end", 1, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "negotiate_start", 2, false, zapcore.InfoLevel)
	checkLogFields(t, logs, "negotiate_end", 2, true, zapcore.ErrorLevel)
}

func TestSSHTransportRequiresLogger(t *testing.T) {
	if _, err := New(transport.Config{SSHUser: "u", SSHPassword: "p"}); err == nil {
		t.Fatalf("expected error when logger is nil")
	}
}
