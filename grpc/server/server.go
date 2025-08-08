package server

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"os"

	lvmagent "lvmsync_go/internal/lvm"
	"lvmsync_go/proto"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type Config struct {
	TLSCert       string
	TLSKey        string
	CACert        string
	AllowInsecure bool
}

func New(conf Config, a lvmagent.Agent) *grpc.Server {
	opts := []grpc.ServerOption{
		grpc.UnaryInterceptor(authorizeInterceptor),
	}

	if !conf.AllowInsecure {
		cert, err := tls.LoadX509KeyPair(conf.TLSCert, conf.TLSKey)
		if err != nil {
			zap.L().Fatal("load TLS key pair", zap.Error(err))
		}
		caPEM, err := os.ReadFile(conf.CACert)
		if err != nil {
			zap.L().Fatal("read CA cert", zap.Error(err))
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caPEM) {
			zap.L().Fatal("invalid CA cert")
		}
		tlsCfg := &tls.Config{
			Certificates: []tls.Certificate{cert},
			ClientAuth:   tls.RequireAndVerifyClientCert,
			ClientCAs:    pool,
			MinVersion:   tls.VersionTLS13,
		}
		opts = append(opts, grpc.Creds(credentials.NewTLS(tlsCfg)))
	}

	srv := grpc.NewServer(opts...)
	proto.RegisterReplicationServer(srv, &replicationServer{agent: a})
	return srv
}

func authorizeInterceptor(ctx context.Context, req interface{}, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
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

type replicationServer struct {
	proto.UnimplementedReplicationServer
	agent lvmagent.Agent
}

func (s *replicationServer) LockVolume(ctx context.Context, req *proto.LockRequest) (*proto.StatusResponse, error) {
	if s.agent == nil {
		return &proto.StatusResponse{Ok: false, Message: "agent not configured"}, nil
	}
	if err := s.agent.Lock(ctx, req.GetVolumeName(), req.GetRequester()); err != nil {
		return &proto.StatusResponse{Ok: false, Message: err.Error()}, nil
	}
	return &proto.StatusResponse{Ok: true}, nil
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
	if s.agent == nil {
		return &proto.StatusResponse{Ok: false, Message: "agent not configured"}, nil
	}
	err := s.agent.SendMetadata(ctx, lvmagent.VolumeMetadata{VolumeName: md.GetVolumeName(), SizeBytes: md.GetSizeBytes(), ChunkSize: md.GetChunkSize()})
	if err != nil {
		return &proto.StatusResponse{Ok: false, Message: err.Error()}, nil
	}
	return &proto.StatusResponse{Ok: true}, nil
}

func (s *replicationServer) StartTransferSession(ctx context.Context, req *proto.LockRequest) (*proto.StatusResponse, error) {
	if s.agent == nil {
		return &proto.StatusResponse{Ok: false, Message: "agent not configured"}, nil
	}
	if err := s.agent.StartTransferSession(ctx, req.GetVolumeName(), req.GetRequester()); err != nil {
		return &proto.StatusResponse{Ok: false, Message: err.Error()}, nil
	}
	return &proto.StatusResponse{Ok: true}, nil
}

func (s *replicationServer) FinalizeSync(ctx context.Context, req *proto.LockRequest) (*proto.StatusResponse, error) {
	if s.agent == nil {
		return &proto.StatusResponse{Ok: false, Message: "agent not configured"}, nil
	}
	if err := s.agent.FinalizeSync(ctx, req.GetVolumeName(), req.GetRequester()); err != nil {
		return &proto.StatusResponse{Ok: false, Message: err.Error()}, nil
	}
	if err := s.agent.Unlock(ctx, req.GetVolumeName(), req.GetRequester()); err != nil {
		return &proto.StatusResponse{Ok: false, Message: err.Error()}, nil
	}
	return &proto.StatusResponse{Ok: true}, nil
}

func (s *replicationServer) GetStatus(ctx context.Context, req *proto.LockRequest) (*proto.StatusResponse, error) {
	if s.agent == nil {
		return &proto.StatusResponse{Ok: false, Message: "agent not configured"}, nil
	}
	msg, err := s.agent.GetStatus(ctx, req.GetVolumeName(), req.GetRequester())
	if err != nil {
		return &proto.StatusResponse{Ok: false, Message: err.Error()}, nil
	}
	return &proto.StatusResponse{Ok: true, Message: msg}, nil
}

func (s *replicationServer) Ping(_ context.Context, _ *proto.Empty) (*proto.StatusResponse, error) {
	return &proto.StatusResponse{Ok: true, Message: "pong"}, nil
}
