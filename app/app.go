package app

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc"

	signalspkg "lvmsync_go/cmd/lvmsync/signals"
	"lvmsync_go/config"
	grpcclient "lvmsync_go/grpc/client"
	grpcserver "lvmsync_go/grpc/server"
	clientpkg "lvmsync_go/internal/client"
	lvmlib "lvmsync_go/internal/lvm"
	"lvmsync_go/proto"
)

// Wrappers for external dependencies to enable test stubbing.
var (
	listen    = net.Listen
	newServer = func(cfg grpcserver.Config, agent lvmlib.Agent) (grpcServer, error) {
		return grpcserver.New(cfg, agent)
	}
	dial = func(ctx context.Context, addr string, conf grpcclient.Config) (closeableConn, error) {
		return grpcclient.Dial(ctx, addr, conf)
	}
	handshake     = grpcclient.Handshake
	createSession = grpcclient.CreateSession
	ackStream     = func(ctx context.Context, c proto.ReplicationClient, id string) (ackStreamClient, error) {
		return grpcclient.AckStream(ctx, c, id)
	}
	handleSignals     = signalspkg.Handle
	prepareSnapshot   = clientpkg.PrepareSnapshot
	newTicker         = time.NewTicker
	heartbeatInterval = 30 * time.Second
)

type grpcServer interface {
	Serve(net.Listener) error
	GracefulStop()
}

type closeableConn interface {
	grpc.ClientConnInterface
	Close() error
}

type ackStreamClient interface {
	Send(*proto.Ack) error
	CloseSend() error
}

// StartGRPCServer starts the gRPC server if configured and returns a cleanup function and error channel.
func StartGRPCServer(ctx context.Context, cfg *config.Config, logger *zap.Logger) (func(), <-chan error, error) {
	errCh := make(chan error, 1)
	if cfg.GRPCListen == "" {
		close(errCh)
		return func() {}, errCh, nil
	}

	srvCfg := grpcserver.Config{TLSCert: cfg.TLSCert, TLSKey: cfg.TLSKey, CACert: cfg.CACert, AllowInsecure: cfg.AllowInsecure}
	ln, err := listen("tcp", cfg.GRPCListen)
	if err != nil {
		return nil, nil, fmt.Errorf("gRPC listen: %w", err)
	}
	srv, err := newServer(srvCfg, nil)
	if err != nil {
		ln.Close()
		return nil, nil, fmt.Errorf("gRPC server: %w", err)
	}
	go func() {
		serveErrCh := make(chan error, 1)
		go func() { serveErrCh <- srv.Serve(ln) }()
		var serveErr error
		select {
		case serveErr = <-serveErrCh:
		case <-ctx.Done():
			srv.GracefulStop()
			ln.Close()
			serveErr = <-serveErrCh
		}
		errCh <- serveErr
	}()
	cleanup := func() {
		srv.GracefulStop()
		ln.Close()
	}
	return cleanup, errCh, nil
}

// ClientHandshake performs the gRPC client handshake and returns a cleanup function and heartbeat error channel.
func ClientHandshake(cfg *config.Config, logger *zap.Logger) (func(), chan error, error) {
	if cfg.GRPCConnect == "" {
		return func() {}, nil, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	conn, err := dial(ctx, cfg.GRPCConnect, grpcclient.Config{
		TLSCert:       cfg.TLSCert,
		TLSKey:        cfg.TLSKey,
		CACert:        cfg.CACert,
		AllowInsecure: cfg.AllowInsecure,
		DialTimeout:   cfg.GRPCDialTimeout,
	})
	if err != nil {
		cancel()
		return nil, nil, fmt.Errorf("gRPC dial: %w", err)
	}
	c := proto.NewReplicationClient(conn)
	hs := &proto.HandshakeRequest{SectorSize: 512, Alignment: 512, MaxConcurrency: uint32(cfg.Parallel), DedupSupported: true, CompressionSupported: true}
	hsCtx, hsCancel := context.WithTimeout(ctx, 10*time.Second)
	if _, err := handshake(hsCtx, c, hs); err != nil {
		hsCancel()
		cancel()
		conn.Close()
		return nil, nil, fmt.Errorf("handshake failed: %w", err)
	}
	sess, err := createSession(hsCtx, c, "vol", "dev")
	hsCancel()
	if err != nil {
		cancel()
		conn.Close()
		return nil, nil, fmt.Errorf("create session: %w", err)
	}
	stream, err := ackStream(ctx, c, sess.GetSessionId())
	if err != nil {
		cancel()
		conn.Close()
		return nil, nil, fmt.Errorf("ack stream: %w", err)
	}
	hbErrCh := make(chan error, 1)
	go func(id string) {
		ticker := newTicker(heartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := stream.Send(&proto.Ack{SessionId: id, Ok: true, Message: "ping"}); err != nil {
					logger.Error("heartbeat send failed", zap.Error(err))
					hbErrCh <- err
					cancel()
					return
				}
			}
		}
	}(sess.GetSessionId())
	cleanup := func() {
		cancel()
		stream.CloseSend()
		conn.Close()
		close(hbErrCh)
	}
	return cleanup, hbErrCh, nil
}

// SetupSignalHandling configures signal handling and returns the signal and error channels.
func SetupSignalHandling(cfg *config.Config, snapshotPath *string, logger *zap.Logger) (chan os.Signal, chan error) {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	sigErrCh := make(chan error, 1)
	go handleSignals(cfg, logger, signals, snapshotPath, sigErrCh)
	return signals, sigErrCh
}

// PrepareSnapshot wraps the snapshot preparation logic.
func PrepareSnapshot(cfg *config.Config, originalVolume string, logger *zap.Logger) (string, chan error, func(), error) {
	return prepareSnapshot(cfg, originalVolume, logger)
}
