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
	grpcclient "lvmsync_go/grpc/client"
	grpcserver "lvmsync_go/grpc/server"
	clientpkg "lvmsync_go/internal/client"
	"lvmsync_go/internal/config"
	lvmlib "lvmsync_go/internal/lvm"
	"lvmsync_go/proto"
)

// Runner holds dependencies for application helpers.
type Runner struct {
	listen          func(network, addr string) (net.Listener, error)
	newServer       func(cfg grpcserver.Config, agent lvmlib.Agent, logger *zap.Logger) (grpcServer, func(), error)
	dial            func(ctx context.Context, addr string, conf grpcclient.Config, logger *zap.Logger) (closeableConn, error)
	handshake       func(ctx context.Context, c proto.ReplicationClient, hs *proto.HandshakeRequest) (*proto.HandshakeResponse, error)
	createSession   func(ctx context.Context, c proto.ReplicationClient, vol, dev string) (*proto.SessionResponse, error)
	ackStream       func(ctx context.Context, c proto.ReplicationClient, id string) (ackStreamClient, error)
	signalsHandler  signalspkg.Handler
	prepareSnapshot func(context.Context, *config.Config, string, *zap.Logger) (string, chan error, func(), error)
	newTicker       func(time.Duration) *time.Ticker
}

// NewRunner constructs a Runner with production dependencies.
func NewRunner() *Runner {
	return &Runner{
		listen: net.Listen,
		newServer: func(cfg grpcserver.Config, agent lvmlib.Agent, logger *zap.Logger) (grpcServer, func(), error) {
			return grpcserver.New(cfg, agent, logger)
		},
		dial: func(ctx context.Context, addr string, conf grpcclient.Config, logger *zap.Logger) (closeableConn, error) {
			return grpcclient.Dial(ctx, addr, conf, logger)
		},
		handshake:     grpcclient.Handshake,
		createSession: grpcclient.CreateSession,
		ackStream: func(ctx context.Context, c proto.ReplicationClient, id string) (ackStreamClient, error) {
			return grpcclient.AckStream(ctx, c, id)
		},
		signalsHandler:  signalspkg.NewRunner(),
		prepareSnapshot: clientpkg.PrepareSnapshot,
		newTicker:       time.NewTicker,
	}
}

// NewRunnerWithDeps constructs a Runner overriding default dependencies.
func NewRunnerWithDeps(deps *Runner) *Runner {
	r := NewRunner()
	if deps == nil {
		return r
	}
	if deps.listen != nil {
		r.listen = deps.listen
	}
	if deps.newServer != nil {
		r.newServer = deps.newServer
	}
	if deps.dial != nil {
		r.dial = deps.dial
	}
	if deps.handshake != nil {
		r.handshake = deps.handshake
	}
	if deps.createSession != nil {
		r.createSession = deps.createSession
	}
	if deps.ackStream != nil {
		r.ackStream = deps.ackStream
	}
	if deps.signalsHandler != nil {
		r.signalsHandler = deps.signalsHandler
	}
	if deps.prepareSnapshot != nil {
		r.prepareSnapshot = deps.prepareSnapshot
	}
	if deps.newTicker != nil {
		r.newTicker = deps.newTicker
	}
	return r
}

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
func (r *Runner) StartGRPCServer(ctx context.Context, cfg *config.Config, logger *zap.Logger) (func(), <-chan error, error) {
	errCh := make(chan error, 1)
	if cfg.GRPCListen == "" {
		close(errCh)
		return func() {}, errCh, nil
	}

	srvCfg := grpcserver.Config{TLSCert: cfg.TLSCert, TLSKey: cfg.TLSKey, CACert: cfg.CACert, AllowInsecure: cfg.AllowInsecure}
	ln, err := r.listen("tcp", cfg.GRPCListen)
	if err != nil {
		return nil, nil, fmt.Errorf("gRPC listen: %w", err)
	}
	srv, srvCleanup, err := r.newServer(srvCfg, nil, logger)
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
		srvCleanup()
		srv.GracefulStop()
		ln.Close()
		close(errCh)
	}
	return cleanup, errCh, nil
}

// ClientHandshake performs the gRPC client handshake and returns a cleanup function and heartbeat error channel.
func (r *Runner) ClientHandshake(ctx context.Context, cfg *config.Config, logger *zap.Logger) (func(), chan error, error) {
	if cfg.GRPCConnect == "" {
		return func() {}, nil, nil
	}
	ctx, cancel := context.WithCancel(ctx)
	conn, err := r.dial(ctx, cfg.GRPCConnect, grpcclient.Config{
		TLSCert:       cfg.TLSCert,
		TLSKey:        cfg.TLSKey,
		CACert:        cfg.CACert,
		AllowInsecure: cfg.AllowInsecure,
		DialTimeout:   cfg.GRPCDialTimeout,
	}, logger)
	if err != nil {
		cancel()
		return nil, nil, fmt.Errorf("gRPC dial: %w", err)
	}
	c := proto.NewReplicationClient(conn)
	hs := &proto.HandshakeRequest{SectorSize: 512, Alignment: 512, MaxConcurrency: uint32(cfg.Parallel), DedupSupported: true, CompressionSupported: true}
	hsCtx, hsCancel := context.WithTimeout(ctx, 10*time.Second)
	if _, err := r.handshake(hsCtx, c, hs); err != nil {
		hsCancel()
		cancel()
		conn.Close()
		return nil, nil, fmt.Errorf("handshake failed: %w", err)
	}
	sess, err := r.createSession(hsCtx, c, "vol", "dev")
	hsCancel()
	if err != nil {
		cancel()
		conn.Close()
		return nil, nil, fmt.Errorf("create session: %w", err)
	}
	stream, err := r.ackStream(ctx, c, sess.GetSessionId())
	if err != nil {
		cancel()
		conn.Close()
		return nil, nil, fmt.Errorf("ack stream: %w", err)
	}
	hbErrCh := make(chan error, 1)
	go func(id string) {
		ticker := r.newTicker(cfg.HeartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				sendCtx, sendCancel := context.WithTimeout(ctx, cfg.HeartbeatSendTimeout)
				errCh := make(chan error, 1)
				go func() {
					errCh <- stream.Send(&proto.Ack{SessionId: id, Ok: true, Message: "ping"})
				}()
				select {
				case err := <-errCh:
					sendCancel()
					if err != nil {
						logger.Error("heartbeat send failed", zap.Error(err))
						hbErrCh <- err
						cancel()
						return
					}
				case <-sendCtx.Done():
					sendCancel()
					logger.Error("heartbeat send timeout", zap.Error(sendCtx.Err()))
					hbErrCh <- sendCtx.Err()
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
func (r *Runner) SetupSignalHandling(ctx context.Context, cfg *config.Config, snapshotPath *string, logger *zap.Logger) (chan os.Signal, chan error) {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	sigErrCh := make(chan error, 1)
	go r.signalsHandler.Handle(ctx, cfg, logger, signals, snapshotPath, sigErrCh)
	return signals, sigErrCh
}

// PrepareSnapshot wraps the snapshot preparation logic.
func (r *Runner) PrepareSnapshot(ctx context.Context, cfg *config.Config, originalVolume string, logger *zap.Logger) (string, chan error, func(), error) {
	return r.prepareSnapshot(ctx, cfg, originalVolume, logger)
}

// StartGRPCServer starts the gRPC server with default dependencies.
func StartGRPCServer(ctx context.Context, cfg *config.Config, logger *zap.Logger) (func(), <-chan error, error) {
	return NewRunner().StartGRPCServer(ctx, cfg, logger)
}

// ClientHandshake performs the handshake with default dependencies.
func ClientHandshake(ctx context.Context, cfg *config.Config, logger *zap.Logger) (func(), chan error, error) {
	return NewRunner().ClientHandshake(ctx, cfg, logger)
}

// SetupSignalHandling configures signal handling with default dependencies.
func SetupSignalHandling(ctx context.Context, cfg *config.Config, snapshotPath *string, logger *zap.Logger) (chan os.Signal, chan error) {
	return NewRunner().SetupSignalHandling(ctx, cfg, snapshotPath, logger)
}

// PrepareSnapshot wraps snapshot preparation with default dependencies.
func PrepareSnapshot(ctx context.Context, cfg *config.Config, originalVolume string, logger *zap.Logger) (string, chan error, func(), error) {
	return NewRunner().PrepareSnapshot(ctx, cfg, originalVolume, logger)
}
