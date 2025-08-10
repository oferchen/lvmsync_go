package client

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/test/bufconn"

	servers "lvmsync_go/grpc/server"
	"lvmsync_go/proto"
)

const bufSize = 1024 * 1024

func bufDialer(lis *bufconn.Listener) func(context.Context, string) (net.Conn, error) {
	return func(ctx context.Context, s string) (net.Conn, error) {
		return lis.Dial()
	}
}

func setupClient(t *testing.T) (proto.ReplicationClient, func()) {
	t.Helper()
	lis := bufconn.Listen(bufSize)
	srv, err := servers.New(servers.Config{AllowInsecure: true}, nil)
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}
	go srv.Serve(lis)
	conn, err := Dial("bufnet", Config{AllowInsecure: true}, grpc.WithContextDialer(bufDialer(lis)))
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	cleanup := func() {
		conn.Close()
		srv.Stop()
	}
	return proto.NewReplicationClient(conn), cleanup
}

func TestHandshakeAndAck(t *testing.T) {
	client, cleanup := setupClient(t)
	defer cleanup()
	ctx := metadata.NewOutgoingContext(context.Background(), metadata.Pairs("role", "replicator"))
	if _, err := Handshake(ctx, client, &proto.HandshakeRequest{SectorSize: 512, Alignment: 512, MaxConcurrency: 1}); err != nil {
		t.Fatalf("Handshake: %v", err)
	}
	sess, err := client.CreateSession(ctx, &proto.SessionRequest{VolumeName: "vol", DeviceUuid: "dev", ClientCert: dummyCert(t)})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	stream, err := AckStream(ctx, client, sess.GetSessionId())
	if err != nil {
		t.Fatalf("AckStream: %v", err)
	}
	if err := stream.Send(&proto.Ack{SessionId: sess.GetSessionId(), Ok: true, Message: "ping"}); err != nil {
		t.Fatalf("send: %v", err)
	}
	if _, err := stream.Recv(); err != nil {
		t.Fatalf("recv: %v", err)
	}
}

func dummyCert(t *testing.T) []byte {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test"}, NotBefore: time.Now(), NotAfter: time.Now().Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("CreateCertificate: %v", err)
	}
	return der
}
