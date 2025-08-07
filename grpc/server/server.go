package server

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"os"

	"lvmsync_go/lvm"
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

func New(conf Config, l lvm.API) *grpc.Server {
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
	proto.RegisterReplicationServer(srv, &replicationServer{lvm: l})
	return srv
}

func authorizeInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
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
	lvm lvm.API
}
