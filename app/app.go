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
	dial = func(addr string, conf grpcclient.Config) (closeableConn, error) {
		return grpcclient.Dial(addr, conf)
	}
	handshake     = grpcclient.Handshake
	createSession = grpcclient.CreateSession
	ackStream     = func(ctx context.Context, c proto.ReplicationClient, id string) (ackStreamClient, error) {
		return grpcclient.AckStream(ctx, c, id)
	}
	handleSignals   = signalspkg.Handle
	prepareSnapshot = clientpkg.PrepareSnapshot
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

// StartGRPCServer starts the gRPC server if configured and returns a cleanup function.
func StartGRPCServer(cfg *config.Config, logger *zap.Logger) (func(), error) {
	if cfg.GRPCListen == "" {
		return func() {}, nil
	}

	srvCfg := grpcserver.Config{TLSCert: cfg.TLSCert, TLSKey: cfg.TLSKey, CACert: cfg.CACert, AllowInsecure: cfg.AllowInsecure}
	ln, err := listen("tcp", cfg.GRPCListen)
	if err != nil {
		return nil, fmt.Errorf("gRPC listen: %w", err)
	}
	srv, err := newServer(srvCfg, nil)
	if err != nil {
		ln.Close()
		return nil, fmt.Errorf("gRPC server: %w", err)
	}
	go func() {
		if err := srv.Serve(ln); err != nil {
			logger.Error("grpc server", zap.Error(err))
		}
	}()
	cleanup := func() {
		srv.GracefulStop()
		ln.Close()
	}
	return cleanup, nil
}

// ClientHandshake performs the gRPC client handshake and returns a cleanup function.
func ClientHandshake(cfg *config.Config, logger *zap.Logger) (func(), error) {
	if cfg.GRPCConnect == "" {
		return func() {}, nil
	}
	conn, err := dial(cfg.GRPCConnect, grpcclient.Config{
		TLSCert:       cfg.TLSCert,
		TLSKey:        cfg.TLSKey,
		CACert:        cfg.CACert,
		AllowInsecure: cfg.AllowInsecure,
	})
	if err != nil {
		return nil, fmt.Errorf("gRPC dial: %w", err)
	}
	c := proto.NewReplicationClient(conn)
	hs := &proto.HandshakeRequest{SectorSize: 512, Alignment: 512, MaxConcurrency: uint32(cfg.Parallel), DedupSupported: true, CompressionSupported: true}
	if _, err := handshake(context.Background(), c, hs); err != nil {
		conn.Close()
		return nil, fmt.Errorf("handshake failed: %w", err)
	}
	sess, err := createSession(context.Background(), c, "vol", "dev")
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("create session: %w", err)
	}
	stream, err := ackStream(context.Background(), c, sess.GetSessionId())
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("ack stream: %w", err)
	}
	go func(id string) {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			if err := stream.Send(&proto.Ack{SessionId: id, Ok: true, Message: "ping"}); err != nil {
				return
			}
		}
	}(sess.GetSessionId())
	cleanup := func() {
		stream.CloseSend()
		conn.Close()
	}
	return cleanup, nil
}

// SetupSignalHandling configures signal handling and returns the signal and error channels.
func SetupSignalHandling(cfg *config.Config, snapshotPath *string) (chan os.Signal, chan error) {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	sigErrCh := make(chan error, 1)
	go handleSignals(cfg, signals, snapshotPath, sigErrCh)
	return signals, sigErrCh
}

// PrepareSnapshot wraps the snapshot preparation logic.
func PrepareSnapshot(cfg *config.Config, originalVolume string, logger *zap.Logger) (string, chan error, func(), error) {
	return prepareSnapshot(cfg, originalVolume, logger)
}
