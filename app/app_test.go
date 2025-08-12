package app

import (
	"context"
	"errors"
	"net"
	"os"
	"os/signal"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc"

	"lvmsync_go/config"
	grpcclient "lvmsync_go/grpc/client"
	grpcserver "lvmsync_go/grpc/server"
	lvmlib "lvmsync_go/internal/lvm"
	"lvmsync_go/proto"
)

// fake implementations for testing

type fakeListener struct{}

func (f *fakeListener) Accept() (net.Conn, error) { return nil, errors.New("accept not implemented") }
func (f *fakeListener) Close() error              { return nil }
func (f *fakeListener) Addr() net.Addr            { return &net.TCPAddr{} }

type fakeServer struct {
	served  atomic.Bool
	stopped atomic.Bool
}

func (f *fakeServer) Serve(net.Listener) error { f.served.Store(true); return nil }
func (f *fakeServer) GracefulStop()            { f.stopped.Store(true) }

func TestStartGRPCServerSuccess(t *testing.T) {
	cfg := &config.Config{GRPCListen: "127.0.0.1:0"}
	logger := zap.NewNop()
	origListen := listen
	origNewServer := newServer
	defer func() { listen = origListen; newServer = origNewServer }()
	listen = func(network, addr string) (net.Listener, error) { return &fakeListener{}, nil }
	srv := &fakeServer{}
	newServer = func(conf grpcserver.Config, agent lvmlib.Agent) (grpcServer, error) { return srv, nil }

	ctx, cancel := context.WithCancel(context.Background())
	cleanup, errCh, err := StartGRPCServer(ctx, cfg, logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	time.Sleep(10 * time.Millisecond)
	if !srv.served.Load() {
		t.Fatalf("server not started")
	}
	cleanup()
	cancel()
	if !srv.stopped.Load() {
		t.Fatalf("server not stopped")
	}
	if err := <-errCh; err != nil {
		t.Fatalf("unexpected server error: %v", err)
	}
}

func TestStartGRPCServerListenError(t *testing.T) {
	cfg := &config.Config{GRPCListen: "bad"}
	logger := zap.NewNop()
	origListen := listen
	defer func() { listen = origListen }()
	listen = func(network, addr string) (net.Listener, error) { return nil, errors.New("boom") }

	if _, _, err := StartGRPCServer(context.Background(), cfg, logger); err == nil {
		t.Fatalf("expected error")
	}
}

func TestStartGRPCServerServeError(t *testing.T) {
	cfg := &config.Config{GRPCListen: "127.0.0.1:0"}
	logger := zap.NewNop()
	origListen := listen
	origNewServer := newServer
	defer func() { listen = origListen; newServer = origNewServer }()
	listen = func(network, addr string) (net.Listener, error) { return &fakeListener{}, nil }
	srvErr := errors.New("serve boom")
	newServer = func(conf grpcserver.Config, agent lvmlib.Agent) (grpcServer, error) {
		return &failingServer{err: srvErr}, nil
	}
	cleanup, errCh, err := StartGRPCServer(cfg, logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer cleanup()
	select {
	case err := <-errCh:
		if !errors.Is(err, srvErr) {
			t.Fatalf("unexpected serve error: %v", err)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatalf("timeout waiting for serve error")
	}
}

type failingServer struct{ err error }

func (f *failingServer) Serve(net.Listener) error { return f.err }
func (f *failingServer) GracefulStop()            {}

// ClientHandshake tests

type fakeConn struct{ closed bool }

func (f *fakeConn) Invoke(context.Context, string, any, any, ...grpc.CallOption) error { return nil }
func (f *fakeConn) NewStream(context.Context, *grpc.StreamDesc, string, ...grpc.CallOption) (grpc.ClientStream, error) {
	return nil, nil
}
func (f *fakeConn) Close() error { f.closed = true; return nil }

type fakeStream struct{ closed bool }

func (f *fakeStream) Send(*proto.Ack) error { return nil }
func (f *fakeStream) CloseSend() error      { f.closed = true; return nil }

func TestClientHandshakeSuccess(t *testing.T) {
	cfg := &config.Config{GRPCConnect: "addr", Parallel: 1, GRPCDialTimeout: time.Second}
	logger := zap.NewNop()

	fc := &fakeConn{}
	fs := &fakeStream{}
	origDial := dial
	origHandshake := handshake
	origCreateSession := createSession
	origAckStream := ackStream
	defer func() {
		dial = origDial
		handshake = origHandshake
		createSession = origCreateSession
		ackStream = origAckStream
	}()
	dial = func(context.Context, string, grpcclient.Config) (closeableConn, error) { return fc, nil }
	handshake = func(context.Context, proto.ReplicationClient, *proto.HandshakeRequest) (*proto.HandshakeResponse, error) {
		return &proto.HandshakeResponse{}, nil
	}
	createSession = func(context.Context, proto.ReplicationClient, string, string) (*proto.SessionResponse, error) {
		return &proto.SessionResponse{SessionId: "id"}, nil
	}
	ackStream = func(context.Context, proto.ReplicationClient, string) (ackStreamClient, error) { return fs, nil }

	cleanup, hbErrCh, err := ClientHandshake(cfg, logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cleanup()
	if !fc.closed || !fs.closed {
		t.Fatalf("cleanup did not close resources")
	}
	if err, ok := <-hbErrCh; ok || err != nil {
		t.Fatalf("unexpected heartbeat error: %v", err)
	}
}

func TestClientHandshakeDialError(t *testing.T) {
	cfg := &config.Config{GRPCConnect: "addr", GRPCDialTimeout: time.Second}
	logger := zap.NewNop()
	origDial := dial
	defer func() { dial = origDial }()
	dial = func(context.Context, string, grpcclient.Config) (closeableConn, error) {
		return nil, errors.New("dial fail")
	}

	if _, _, err := ClientHandshake(cfg, logger); err == nil {
		t.Fatalf("expected error")
	}
}

func TestClientHandshakeHeartbeatFailure(t *testing.T) {
	cfg := &config.Config{GRPCConnect: "addr", Parallel: 1}
	logger := zap.NewNop()

	fc := &fakeConn{}
	fs := &failingStream{}
	origDial := dial
	origHandshake := handshake
	origCreateSession := createSession
	origAckStream := ackStream
	origInterval := heartbeatInterval
	defer func() {
		dial = origDial
		handshake = origHandshake
		createSession = origCreateSession
		ackStream = origAckStream
		heartbeatInterval = origInterval
	}()
	dial = func(ctx context.Context, addr string, conf grpcclient.Config) (closeableConn, error) {
		return fc, nil
	}
	handshake = func(context.Context, proto.ReplicationClient, *proto.HandshakeRequest) (*proto.HandshakeResponse, error) {
		return &proto.HandshakeResponse{}, nil
	}
	createSession = func(context.Context, proto.ReplicationClient, string, string) (*proto.SessionResponse, error) {
		return &proto.SessionResponse{SessionId: "id"}, nil
	}
	ackStream = func(context.Context, proto.ReplicationClient, string) (ackStreamClient, error) { return fs, nil }
	heartbeatInterval = 10 * time.Millisecond

	cleanup, hbErrCh, err := ClientHandshake(cfg, logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer cleanup()
	select {
	case err := <-hbErrCh:
		if err == nil {
			t.Fatalf("expected heartbeat error")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatalf("timeout waiting for heartbeat error")
	}
}

func TestClientHandshakeHeartbeatTimeout(t *testing.T) {
	cfg := &config.Config{GRPCConnect: "addr", Parallel: 1}
	logger := zap.NewNop()

	fc := &fakeConn{}
	block := make(chan struct{})
	fs := &blockingStream{block: block}
	origDial := dial
	origHandshake := handshake
	origCreateSession := createSession
	origAckStream := ackStream
	origInterval := heartbeatInterval
	origTimeout := heartbeatSendTimeout
	defer func() {
		dial = origDial
		handshake = origHandshake
		createSession = origCreateSession
		ackStream = origAckStream
		heartbeatInterval = origInterval
		heartbeatSendTimeout = origTimeout
	}()
	dial = func(ctx context.Context, addr string, conf grpcclient.Config) (closeableConn, error) {
		return fc, nil
	}
	handshake = func(context.Context, proto.ReplicationClient, *proto.HandshakeRequest) (*proto.HandshakeResponse, error) {
		return &proto.HandshakeResponse{}, nil
	}
	createSession = func(context.Context, proto.ReplicationClient, string, string) (*proto.SessionResponse, error) {
		return &proto.SessionResponse{SessionId: "id"}, nil
	}
	ackStream = func(context.Context, proto.ReplicationClient, string) (ackStreamClient, error) { return fs, nil }
	heartbeatInterval = 10 * time.Millisecond
	heartbeatSendTimeout = 20 * time.Millisecond

	cleanup, hbErrCh, err := ClientHandshake(cfg, logger)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer cleanup()
	select {
	case err := <-hbErrCh:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("expected deadline exceeded, got %v", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("timeout waiting for heartbeat error")
	}
	close(block)
}

type failingStream struct{ closed bool }

func (f *failingStream) Send(*proto.Ack) error { return errors.New("send fail") }
func (f *failingStream) CloseSend() error      { f.closed = true; return nil }

type blockingStream struct {
	closed bool
	block  chan struct{}
}

func (b *blockingStream) Send(*proto.Ack) error { <-b.block; return nil }
func (b *blockingStream) CloseSend() error      { b.closed = true; return nil }

// SetupSignalHandling tests

func TestSetupSignalHandling(t *testing.T) {
	cfg := &config.Config{}
	var path string
	called := make(chan struct{})
	origHandle := handleSignals
	defer func() { handleSignals = origHandle }()
	handleSignals = func(cfg *config.Config, _ *zap.Logger, sigs <-chan os.Signal, p *string, errCh chan<- error) {
		<-sigs
		*p = "set"
		close(called)
	}
	logger := zap.NewNop()
	signals, sigErrCh := SetupSignalHandling(cfg, &path, logger)
	signals <- os.Interrupt
	select {
	case <-called:
	case <-time.After(100 * time.Millisecond):
		t.Fatalf("handler not invoked")
	}
	if path != "set" {
		t.Fatalf("path not set")
	}
	select {
	case err := <-sigErrCh:
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	default:
	}
	signal.Stop(signals)
}

func TestSetupSignalHandlingError(t *testing.T) {
	cfg := &config.Config{}
	var path string
	origHandle := handleSignals
	defer func() { handleSignals = origHandle }()
	handleSignals = func(cfg *config.Config, _ *zap.Logger, sigs <-chan os.Signal, p *string, errCh chan<- error) {
		errCh <- errors.New("boom")
	}
	logger := zap.NewNop()
	_, sigErrCh := SetupSignalHandling(cfg, &path, logger)
	if err := <-sigErrCh; err == nil {
		t.Fatalf("expected error")
	}
}

// PrepareSnapshot tests

func TestPrepareSnapshot(t *testing.T) {
	cfg := &config.Config{}
	logger := zap.NewNop()
	orig := prepareSnapshot
	defer func() { prepareSnapshot = orig }()
	prepareSnapshot = func(c *config.Config, v string, l *zap.Logger) (string, chan error, func(), error) {
		return "snap", nil, func() {}, nil
	}
	snap, ch, cleanup, err := PrepareSnapshot(cfg, "vol", logger)
	if err != nil || snap != "snap" || ch != nil || cleanup == nil {
		t.Fatalf("unexpected result")
	}
}

func TestPrepareSnapshotError(t *testing.T) {
	cfg := &config.Config{}
	logger := zap.NewNop()
	orig := prepareSnapshot
	defer func() { prepareSnapshot = orig }()
	prepareSnapshot = func(c *config.Config, v string, l *zap.Logger) (string, chan error, func(), error) {
		return "", nil, nil, errors.New("fail")
	}
	if _, _, _, err := PrepareSnapshot(cfg, "vol", logger); err == nil {
		t.Fatalf("expected error")
	}
}
