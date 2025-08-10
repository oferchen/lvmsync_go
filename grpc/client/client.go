package client

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	"lvmsync_go/proto"
)

type Config struct {
	TLSCert       string
	TLSKey        string
	CACert        string
	AllowInsecure bool
}

func Dial(addr string, conf Config, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
	if conf.AllowInsecure {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	} else {
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
			RootCAs:      pool,
			MinVersion:   tls.VersionTLS13,
			MaxVersion:   tls.VersionTLS13,
		}
		opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg)))
	}
	return grpc.Dial(addr, opts...)
}

func Handshake(ctx context.Context, c proto.ReplicationClient, caps []string) ([]string, error) {
	resp, err := c.ExchangeCapabilities(ctx, &proto.CapabilitySet{Capabilities: caps})
	if err != nil {
		return nil, err
	}
	return resp.GetCapabilities(), nil
}

func AckStream(ctx context.Context, c proto.ReplicationClient) (proto.Replication_AckStreamClient, error) {
	stream, err := c.AckStream(ctx)
	if err != nil {
		return nil, err
	}
	go func() {
		for {
			if _, err := stream.Recv(); err != nil {
				return
			}
		}
	}()
	return stream, nil
}
