package server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"os"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	lvmagent "lvmsync_go/internal/lvm"
	"lvmsync_go/proto"
)

type Config struct {
	TLSCert       string
	TLSKey        string
	CACert        string
	AllowInsecure bool
}

// New constructs a gRPC server and returns it along with a cleanup function
// that flushes logger buffers on shutdown.
func New(conf Config, a lvmagent.Agent, logger *zap.Logger) (*grpc.Server, func(), error) {
	opts := []grpc.ServerOption{
		grpc.UnaryInterceptor(authorizeInterceptor),
		grpc.StreamInterceptor(authorizeStreamInterceptor),
	}

	if !conf.AllowInsecure {
		if conf.TLSCert == "" || conf.TLSKey == "" || conf.CACert == "" {
			return nil, nil, fmt.Errorf("TLSCert, TLSKey, and CACert must be provided when AllowInsecure is false")
		}
		cert, err := tls.LoadX509KeyPair(conf.TLSCert, conf.TLSKey)
		if err != nil {
			return nil, nil, fmt.Errorf("load TLS key pair: %w", err)
		}
		caPEM, err := os.ReadFile(conf.CACert)
		if err != nil {
			return nil, nil, fmt.Errorf("read CA cert: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caPEM) {
			return nil, nil, fmt.Errorf("invalid CA cert")
		}
		tlsCfg := &tls.Config{
			Certificates: []tls.Certificate{cert},
			ClientAuth:   tls.RequireAndVerifyClientCert,
			ClientCAs:    pool,
			MinVersion:   tls.VersionTLS13,
			MaxVersion:   tls.VersionTLS13,
		}
		opts = append(opts, grpc.Creds(credentials.NewTLS(tlsCfg)))
	}

	if logger == nil {
		logger = zap.NewNop()
	}

	srv := grpc.NewServer(opts...)
	proto.RegisterReplicationServer(srv, &replicationServer{agent: a, logger: logger})
	cleanup := func() {
		_ = logger.Sync()
	}
	return srv, cleanup, nil
}

func authorizeInterceptor(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, status.Error(codes.PermissionDenied, "missing metadata")
	}
	roles := md.Get("role")
	for _, r := range roles {
		if r == "replicator" {
			return handler(ctx, req)
		}
	}
	return nil, status.Error(codes.PermissionDenied, "unauthorized role")
}

func authorizeStreamInterceptor(srv any, ss grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	md, ok := metadata.FromIncomingContext(ss.Context())
	if !ok {
		return status.Error(codes.PermissionDenied, "missing metadata")
	}
	roles := md.Get("role")
	for _, r := range roles {
		if r == "replicator" {
			return handler(srv, ss)
		}
	}
	return status.Error(codes.PermissionDenied, "unauthorized role")
}

type replicationServer struct {
	proto.UnimplementedReplicationServer
	agent  lvmagent.Agent
	logger *zap.Logger
}

func (s *replicationServer) agentAction(fn func(lvmagent.Agent) error) *proto.StatusResponse {
	if s.agent == nil {
		return &proto.StatusResponse{Ok: false, Message: "agent not configured"}
	}
	if err := fn(s.agent); err != nil {
		return &proto.StatusResponse{Ok: false, Message: err.Error()}
	}
	return &proto.StatusResponse{Ok: true}
}

func (s *replicationServer) agentStatus(fn func(lvmagent.Agent) (string, error)) *proto.StatusResponse {
	if s.agent == nil {
		return &proto.StatusResponse{Ok: false, Message: "agent not configured"}
	}
	msg, err := fn(s.agent)
	if err != nil {
		return &proto.StatusResponse{Ok: false, Message: err.Error()}
	}
	return &proto.StatusResponse{Ok: true, Message: msg}
}

func (s *replicationServer) LockVolume(ctx context.Context, req *proto.LockRequest) (*proto.StatusResponse, error) {
	return s.agentAction(func(a lvmagent.Agent) error {
		return a.Lock(ctx, req.GetVolumeName(), req.GetRequester())
	}), nil
}

func (s *replicationServer) GetVolumeMetadata(ctx context.Context, req *proto.LockRequest) (*proto.VolumeMetadata, error) {
	if s.agent == nil {
		return nil, status.Error(codes.FailedPrecondition, "agent not configured")
	}
	md, err := s.agent.GetMetadata(ctx, req.GetVolumeName())
	if err != nil {
		return nil, err
	}
	return &proto.VolumeMetadata{VolumeName: md.VolumeName, SizeBytes: md.SizeBytes, ChunkSize: md.ChunkSize}, nil
}

func (s *replicationServer) SendVolumeMetadata(ctx context.Context, md *proto.VolumeMetadata) (*proto.StatusResponse, error) {
	return s.agentAction(func(a lvmagent.Agent) error {
		return a.SendMetadata(ctx, lvmagent.VolumeMetadata{VolumeName: md.GetVolumeName(), SizeBytes: md.GetSizeBytes(), ChunkSize: md.GetChunkSize()})
	}), nil
}

func (s *replicationServer) StartTransferSession(ctx context.Context, req *proto.LockRequest) (*proto.StatusResponse, error) {
	return s.agentAction(func(a lvmagent.Agent) error {
		return a.StartTransferSession(ctx, req.GetVolumeName(), req.GetRequester())
	}), nil
}

func (s *replicationServer) FinalizeSync(ctx context.Context, req *proto.LockRequest) (*proto.StatusResponse, error) {
	return s.agentAction(func(a lvmagent.Agent) error {
		if err := a.FinalizeSync(ctx, req.GetVolumeName(), req.GetRequester()); err != nil {
			return err
		}
		return a.Unlock(ctx, req.GetVolumeName(), req.GetRequester())
	}), nil
}

func (s *replicationServer) GetStatus(ctx context.Context, req *proto.LockRequest) (*proto.StatusResponse, error) {
	return s.agentStatus(func(a lvmagent.Agent) (string, error) {
		return a.GetStatus(ctx, req.GetVolumeName(), req.GetRequester())
	}), nil
}

func (s *replicationServer) Ping(_ context.Context, _ *proto.Empty) (*proto.StatusResponse, error) {
	return &proto.StatusResponse{Ok: true, Message: "pong"}, nil
}

func (s *replicationServer) Handshake(_ context.Context, req *proto.HandshakeRequest) (*proto.HandshakeResponse, error) {
	if req.GetSectorSize() == 0 || req.GetAlignment() == 0 {
		return nil, status.Error(codes.InvalidArgument, "missing handshake parameters")
	}
	return &proto.HandshakeResponse{Ok: true, Message: "handshake ok"}, nil
}

func (s *replicationServer) CreateSession(_ context.Context, req *proto.SessionRequest) (*proto.SessionResponse, error) {
	if _, err := x509.ParseCertificate(req.GetClientCert()); err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid client cert")
	}
	sessionID := req.GetVolumeName() + "-session"
	seed := make([]byte, 32)
	if _, err := rand.Read(seed); err != nil {
		return nil, err
	}
	h := sha256.New()
	h.Write([]byte(sessionID))
	h.Write([]byte(req.GetDeviceUuid()))
	h.Write(seed)
	psk := h.Sum(nil)
	return &proto.SessionResponse{SessionId: sessionID, Psk: psk}, nil
}

func (s *replicationServer) SendResumeBitmap(stream proto.Replication_SendResumeBitmapServer) error {
	if s.agent == nil {
		return status.Error(codes.FailedPrecondition, "agent not configured")
	}
	a, ok := s.agent.(interface {
		SendResumeBitmap(ctx context.Context, sessionID string, bitmap []byte) error
	})
	if !ok {
		return status.Error(codes.Unimplemented, "SendResumeBitmap not supported")
	}
	ctx := stream.Context()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		msg, err := stream.Recv()
		if err == io.EOF {
			return stream.SendAndClose(&proto.StatusResponse{Ok: true})
		}
		if err != nil {
			return err
		}
		s.logger.Debug("resume_bitmap_recv",
			zap.String("session_id", msg.GetSessionId()),
			zap.Int("size_bytes", len(msg.GetBitmap())),
		)
		if err := a.SendResumeBitmap(ctx, msg.GetSessionId(), msg.GetBitmap()); err != nil {
			return err
		}
	}
}

func (s *replicationServer) SendFinalManifest(ctx context.Context, m *proto.ManifestMessage) (*proto.StatusResponse, error) {
	if s.agent == nil {
		return &proto.StatusResponse{Ok: false, Message: "agent not configured"}, nil
	}
	a, ok := s.agent.(interface {
		SendFinalManifest(ctx context.Context, sessionID string, manifest []byte) error
	})
	if !ok {
		return &proto.StatusResponse{Ok: false, Message: "SendFinalManifest not supported"}, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.logger.Info("final_manifest_recv",
		zap.String("session_id", m.GetSessionId()),
		zap.Int("size_bytes", len(m.GetManifest())),
	)
	if err := a.SendFinalManifest(ctx, m.GetSessionId(), m.GetManifest()); err != nil {
		return &proto.StatusResponse{Ok: false, Message: err.Error()}, nil
	}
	return &proto.StatusResponse{Ok: true}, nil
}

func (s *replicationServer) Finalize(ctx context.Context, req *proto.FinalizeRequest) (*proto.StatusResponse, error) {
	if s.agent == nil {
		return &proto.StatusResponse{Ok: false, Message: "agent not configured"}, nil
	}
	a, ok := s.agent.(interface {
		Finalize(ctx context.Context, sessionID string) error
	})
	if !ok {
		return &proto.StatusResponse{Ok: false, Message: "Finalize not supported"}, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.logger.Info("finalize_request", zap.String("session_id", req.GetSessionId()))
	if err := a.Finalize(ctx, req.GetSessionId()); err != nil {
		return &proto.StatusResponse{Ok: false, Message: err.Error()}, nil
	}
	return &proto.StatusResponse{Ok: true}, nil
}

func (s *replicationServer) AckStream(stream proto.Replication_AckStreamServer) error {
	if s.agent == nil {
		return status.Error(codes.FailedPrecondition, "agent not configured")
	}
	a, ok := s.agent.(interface {
		Ack(ctx context.Context, ack *proto.Ack) (*proto.Ack, error)
	})
	if !ok {
		return status.Error(codes.Unimplemented, "Ack not supported")
	}
	ctx := stream.Context()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		msg, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		s.logger.Debug("ack_recv",
			zap.String("session_id", msg.GetSessionId()),
			zap.Bool("ok", msg.GetOk()),
		)
		resp, err := a.Ack(ctx, msg)
		if err != nil {
			return err
		}
		if err := stream.Send(resp); err != nil {
			return err
		}
	}
}

func (s *replicationServer) Probe(ctx context.Context, req *proto.ProbeRequest) (*proto.StatusResponse, error) {
	if s.agent == nil {
		return &proto.StatusResponse{Ok: false, Message: "agent not configured"}, nil
	}
	a, ok := s.agent.(interface {
		Probe(ctx context.Context, volume string) error
	})
	if !ok {
		return &proto.StatusResponse{Ok: false, Message: "Probe not supported"}, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.logger.Info("probe_request", zap.String("volume_name", req.GetVolumeName()))
	if err := a.Probe(ctx, req.GetVolumeName()); err != nil {
		return &proto.StatusResponse{Ok: false, Message: err.Error()}, nil
	}
	return &proto.StatusResponse{Ok: true}, nil
}

func (s *replicationServer) StartSync(ctx context.Context, req *proto.StartSyncRequest) (*proto.StatusResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.logger.Info("start_sync",
		zap.String("volume_name", req.GetVolumeName()),
		zap.String("requester", req.GetRequester()),
	)
	return s.agentAction(func(a lvmagent.Agent) error {
		return a.StartTransferSession(ctx, req.GetVolumeName(), req.GetRequester())
	}), nil
}

func (s *replicationServer) Cancel(ctx context.Context, req *proto.CancelRequest) (*proto.StatusResponse, error) {
	if s.agent == nil {
		return &proto.StatusResponse{Ok: false, Message: "agent not configured"}, nil
	}
	a, ok := s.agent.(interface {
		Cancel(ctx context.Context, sessionID string) error
	})
	if !ok {
		return &proto.StatusResponse{Ok: false, Message: "Cancel not supported"}, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.logger.Info("cancel_request", zap.String("session_id", req.GetSessionId()))
	if err := a.Cancel(ctx, req.GetSessionId()); err != nil {
		return &proto.StatusResponse{Ok: false, Message: err.Error()}, nil
	}
	return &proto.StatusResponse{Ok: true}, nil
}

func (s *replicationServer) ProgressStream(req *proto.ProgressRequest, stream proto.Replication_ProgressStreamServer) error {
	if s.agent == nil {
		return status.Error(codes.FailedPrecondition, "agent not configured")
	}
	a, ok := s.agent.(interface {
		Progress(ctx context.Context, sessionID string) (<-chan *proto.Progress, error)
	})
	if !ok {
		return status.Error(codes.Unimplemented, "Progress not supported")
	}
	ctx := stream.Context()
	if err := ctx.Err(); err != nil {
		return err
	}
	ch, err := a.Progress(ctx, req.GetSessionId())
	if err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case p, ok := <-ch:
			if !ok {
				return nil
			}
			s.logger.Debug("progress_update",
				zap.String("session_id", p.GetSessionId()),
				zap.Uint64("completed", p.GetCompleted()),
				zap.Uint64("total", p.GetTotal()),
			)
			if err := stream.Send(p); err != nil {
				return err
			}
		}
	}
}

func (s *replicationServer) BuildManifest(ctx context.Context, req *proto.BuildManifestRequest) (*proto.StatusResponse, error) {
	if s.agent == nil {
		return &proto.StatusResponse{Ok: false, Message: "agent not configured"}, nil
	}
	a, ok := s.agent.(interface {
		BuildManifest(ctx context.Context, sessionID string) error
	})
	if !ok {
		return &proto.StatusResponse{Ok: false, Message: "BuildManifest not supported"}, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.logger.Info("build_manifest", zap.String("session_id", req.GetSessionId()))
	if err := a.BuildManifest(ctx, req.GetSessionId()); err != nil {
		return &proto.StatusResponse{Ok: false, Message: err.Error()}, nil
	}
	return &proto.StatusResponse{Ok: true}, nil
}

func (s *replicationServer) Verify(ctx context.Context, req *proto.VerifyRequest) (*proto.StatusResponse, error) {
	if s.agent == nil {
		return &proto.StatusResponse{Ok: false, Message: "agent not configured"}, nil
	}
	a, ok := s.agent.(interface {
		Verify(ctx context.Context, sessionID string) error
	})
	if !ok {
		return &proto.StatusResponse{Ok: false, Message: "Verify not supported"}, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.logger.Info("verify_request", zap.String("session_id", req.GetSessionId()))
	if err := a.Verify(ctx, req.GetSessionId()); err != nil {
		return &proto.StatusResponse{Ok: false, Message: err.Error()}, nil
	}
	return &proto.StatusResponse{Ok: true}, nil
}
