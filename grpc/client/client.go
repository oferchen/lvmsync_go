package client

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"os"
	"strings"
	"time"

	"go.uber.org/zap"
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
	DialTimeout   time.Duration
}

func Dial(ctx context.Context, addr string, conf Config, logger *zap.Logger, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
	if logger == nil {
		logger = zap.NewNop()
	}
	if conf.AllowInsecure {
		logger.Warn("allow_insecure_enabled", zap.String("component", "grpc_client"))
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
	target := addr
	if !strings.Contains(addr, "://") {
		target = "passthrough:///" + addr
	}
	ctx, cancel := context.WithTimeout(ctx, conf.DialTimeout)
	defer cancel()
	opts = append(opts, grpc.WithBlock())
	conn, err := grpc.DialContext(ctx, target, opts...)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, err
	}
	return conn, nil
}

func Handshake(ctx context.Context, c proto.ReplicationClient, hs *proto.HandshakeRequest) (*proto.HandshakeResponse, error) {
	return c.Handshake(ctx, hs)
}

func CreateSession(ctx context.Context, c proto.ReplicationClient, volume, device string) (*proto.SessionResponse, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: volume}, NotBefore: time.Now(), NotAfter: time.Now().Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	resp, err := c.CreateSession(ctx, &proto.SessionRequest{VolumeName: volume, DeviceUuid: device, ClientCert: der})
	if err != nil {
		return nil, err
	}
	if _, err := x509.ParseCertificate(resp.GetServerCert()); err != nil {
		return nil, fmt.Errorf("invalid server cert: %w", err)
	}
	if len(resp.GetPsk()) == 0 {
		return nil, fmt.Errorf("missing psk")
	}
	return resp, nil
}

func AckStream(ctx context.Context, c proto.ReplicationClient, sessionID string) (proto.Replication_AckStreamClient, error) {
	stream, err := c.AckStream(ctx)
	if err != nil {
		return nil, err
	}
	if err := stream.Send(&proto.Ack{SessionId: sessionID, Ok: true, Message: "init"}); err != nil {
		return nil, err
	}
	if _, err := stream.Recv(); err != nil {
		return nil, err
	}
	return stream, nil
}

func SendFinalManifest(ctx context.Context, c proto.ReplicationClient, sessionID string, manifest []byte) (*proto.StatusResponse, error) {
	return c.SendFinalManifest(ctx, &proto.ManifestMessage{SessionId: sessionID, Manifest: manifest})
}

func Probe(ctx context.Context, c proto.ReplicationClient, volume string) (*proto.StatusResponse, error) {
	return c.Probe(ctx, &proto.ProbeRequest{VolumeName: volume})
}

func StartSync(ctx context.Context, c proto.ReplicationClient, volume, requester string) (*proto.StatusResponse, error) {
	return c.StartSync(ctx, &proto.StartSyncRequest{VolumeName: volume, Requester: requester})
}

func Cancel(ctx context.Context, c proto.ReplicationClient, sessionID string) (*proto.StatusResponse, error) {
	return c.Cancel(ctx, &proto.CancelRequest{SessionId: sessionID})
}

func Progress(ctx context.Context, c proto.ReplicationClient, sessionID string) (proto.Replication_ProgressStreamClient, error) {
	return c.ProgressStream(ctx, &proto.ProgressRequest{SessionId: sessionID})
}

func BuildManifest(ctx context.Context, c proto.ReplicationClient, sessionID string) (*proto.StatusResponse, error) {
	return c.BuildManifest(ctx, &proto.BuildManifestRequest{SessionId: sessionID})
}

func Verify(ctx context.Context, c proto.ReplicationClient, sessionID string) (*proto.StatusResponse, error) {
	return c.Verify(ctx, &proto.VerifyRequest{SessionId: sessionID})
}
