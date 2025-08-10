package client

import (
	"context"
	"net"
	"testing"

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
	caps, err := Handshake(ctx, client, []string{"foo"})
	if err != nil {
		t.Fatalf("Handshake: %v", err)
	}
	if len(caps) != 1 || caps[0] != "foo" {
		t.Fatalf("unexpected caps %v", caps)
	}
	stream, err := AckStream(ctx, client)
	if err != nil {
		t.Fatalf("AckStream: %v", err)
	}
	if err := stream.Send(&proto.StatusResponse{Ok: true, Message: "ping"}); err != nil {
		t.Fatalf("send: %v", err)
	}
	if _, err := stream.Recv(); err != nil {
		t.Fatalf("recv: %v", err)
	}
}
