package server

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"os"

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

func New(conf Config, a lvmagent.Agent) (*grpc.Server, error) {
	opts := []grpc.ServerOption{
		grpc.UnaryInterceptor(authorizeInterceptor),
	}

	if !conf.AllowInsecure {
		cert, err := tls.LoadX509KeyPair(conf.TLSCert, conf.TLSKey)
		if err != nil {
			return nil, fmt.Errorf("load TLS key pair: %w", err)
		}
		caPEM, err := os.ReadFile(conf.CACert)
		if err != nil {
			return nil, fmt.Errorf("read CA cert: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caPEM) {
			return nil, fmt.Errorf("invalid CA cert")
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
	return srv, nil
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

type replicationServer struct {
	proto.UnimplementedReplicationServer
	agent lvmagent.Agent
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

func (s *replicationServer) ExchangeCapabilities(_ context.Context, caps *proto.CapabilitySet) (*proto.CapabilitySet, error) {
	return caps, nil
}

func (s *replicationServer) CreateSession(_ context.Context, req *proto.SessionRequest) (*proto.SessionResponse, error) {
	return &proto.SessionResponse{SessionId: req.GetVolumeName() + "-session"}, nil
}

func (s *replicationServer) SendResumeBitmap(stream proto.Replication_SendResumeBitmapServer) error {
	for {
		_, err := stream.Recv()
		if err == io.EOF {
			return stream.SendAndClose(&proto.StatusResponse{Ok: true})
		}
		if err != nil {
			return err
		}
	}
}

func (s *replicationServer) Finalize(_ context.Context, req *proto.SessionResponse) (*proto.StatusResponse, error) {
	return &proto.StatusResponse{Ok: true, Message: "finalized " + req.GetSessionId()}, nil
}

func (s *replicationServer) AckStream(stream proto.Replication_AckStreamServer) error {
	for {
		msg, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if err := stream.Send(msg); err != nil {
			return err
		}
	}
}
