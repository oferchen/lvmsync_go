package client

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"net"
	"testing"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/test/bufconn"

	servers "lvmsync_go/grpc/server"
	lvmagent "lvmsync_go/internal/lvm"
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
	agent := ackAgent{}
	srv, srvCleanup, err := servers.New(servers.Config{AllowInsecure: true}, agent, zap.NewNop())
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}
	go srv.Serve(lis)
	ctx, cancel := context.WithCancel(context.Background())
	conn, err := Dial(ctx, "bufnet", Config{AllowInsecure: true, DialTimeout: time.Second}, grpc.WithContextDialer(bufDialer(lis)))
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	cleanup := func() {
		cancel()
		conn.Close()
		srv.Stop()
		srvCleanup()
	}
	return proto.NewReplicationClient(conn), cleanup
}

type ackAgent struct{}

func (ackAgent) Lock(context.Context, string, string) error   { return nil }
func (ackAgent) Unlock(context.Context, string, string) error { return nil }
func (ackAgent) GetMetadata(context.Context, string) (lvmagent.VolumeMetadata, error) {
	return lvmagent.VolumeMetadata{}, nil
}
func (ackAgent) SendMetadata(context.Context, lvmagent.VolumeMetadata) error { return nil }
func (ackAgent) StartTransferSession(context.Context, string, string) error  { return nil }
func (ackAgent) FinalizeSync(context.Context, string, string) error          { return nil }
func (ackAgent) GetStatus(context.Context, string, string) (string, error)   { return "", nil }
func (ackAgent) Ack(ctx context.Context, ack *proto.Ack) (*proto.Ack, error) { return ack, nil }

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

func TestDial(t *testing.T) {
	lis := bufconn.Listen(bufSize)
	srv, srvCleanup, err := servers.New(servers.Config{AllowInsecure: true}, nil, zap.NewNop())
	if err != nil {
		t.Fatalf("server.New: %v", err)
	}
	go srv.Serve(lis)
	defer func() {
		srv.Stop()
		srvCleanup()
	}()

	cases := []struct {
		name    string
		conf    Config
		wantErr bool
	}{
		{"insecure", Config{AllowInsecure: true, DialTimeout: time.Second}, false},
		{"missingCert", Config{TLSCert: "nope", TLSKey: "nope", CACert: "nope", DialTimeout: time.Second}, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts := []grpc.DialOption{}
			if !tc.wantErr {
				opts = append(opts, grpc.WithContextDialer(bufDialer(lis)))
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			conn, err := Dial(ctx, "bufnet", tc.conf, opts...)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("Dial: %v", err)
			}
			conn.Close()
		})
	}
}

func TestDialTimeoutExceeded(t *testing.T) {
	slowDialer := func(ctx context.Context, s string) (net.Conn, error) {
		time.Sleep(50 * time.Millisecond)
		return nil, errors.New("no connection")
	}
	_, err := Dial(context.Background(), "slow", Config{AllowInsecure: true, DialTimeout: 10 * time.Millisecond}, grpc.WithContextDialer(slowDialer))
	if err == nil {
		t.Fatalf("expected error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
}

type fakeSessionClient struct {
	stubClient
	resp *proto.SessionResponse
	err  error
}

func (f *fakeSessionClient) CreateSession(ctx context.Context, _ *proto.SessionRequest, _ ...grpc.CallOption) (*proto.SessionResponse, error) {
	return f.resp, f.err
}

func TestCreateSession(t *testing.T) {
	goodCert := dummyCert(t)
	cases := []struct {
		name    string
		client  proto.ReplicationClient
		wantErr bool
	}{
		{
			name:   "success",
			client: &fakeSessionClient{resp: &proto.SessionResponse{SessionId: "s", Psk: []byte{1}, ServerCert: goodCert}},
		},
		{
			name:    "badCert",
			client:  &fakeSessionClient{resp: &proto.SessionResponse{SessionId: "s", Psk: []byte{1}, ServerCert: []byte("bad")}},
			wantErr: true,
		},
		{
			name:    "missingPSK",
			client:  &fakeSessionClient{resp: &proto.SessionResponse{SessionId: "s", ServerCert: goodCert}},
			wantErr: true,
		},
		{
			name:    "rpcError",
			client:  &fakeSessionClient{err: errors.New("fail")},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := CreateSession(context.Background(), tc.client, "vol", "dev")
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
		})
	}
}

type fakeManifestClient struct {
	stubClient
	resp *proto.StatusResponse
	err  error
}

func (f *fakeManifestClient) SendFinalManifest(ctx context.Context, _ *proto.ManifestMessage, _ ...grpc.CallOption) (*proto.StatusResponse, error) {
	return f.resp, f.err
}

func TestSendFinalManifest(t *testing.T) {
	cases := []struct {
		name    string
		client  proto.ReplicationClient
		wantErr bool
	}{
		{
			name:   "success",
			client: &fakeManifestClient{resp: &proto.StatusResponse{Ok: true}},
		},
		{
			name:    "rpcError",
			client:  &fakeManifestClient{err: errors.New("fail")},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := SendFinalManifest(context.Background(), tc.client, "sess", []byte("{}"))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

type fakeAckClient struct{ stubClient }

func (fakeAckClient) AckStream(context.Context, ...grpc.CallOption) (proto.Replication_AckStreamClient, error) {
	return nil, errors.New("fail")
}

func TestAckStreamError(t *testing.T) {
	c := fakeAckClient{}
	if _, err := AckStream(context.Background(), c, "sess"); err == nil {
		t.Fatalf("expected error")
	}
}

type fakeHandshakeClient struct{ stubClient }

type stubClient struct{}

func (stubClient) LockVolume(context.Context, *proto.LockRequest, ...grpc.CallOption) (*proto.StatusResponse, error) {
	return nil, nil
}

func (stubClient) GetVolumeMetadata(context.Context, *proto.LockRequest, ...grpc.CallOption) (*proto.VolumeMetadata, error) {
	return nil, nil
}

func (stubClient) SendVolumeMetadata(context.Context, *proto.VolumeMetadata, ...grpc.CallOption) (*proto.StatusResponse, error) {
	return nil, nil
}

func (stubClient) StartTransferSession(context.Context, *proto.LockRequest, ...grpc.CallOption) (*proto.StatusResponse, error) {
	return nil, nil
}

func (stubClient) FinalizeSync(context.Context, *proto.LockRequest, ...grpc.CallOption) (*proto.StatusResponse, error) {
	return nil, nil
}

func (stubClient) GetStatus(context.Context, *proto.LockRequest, ...grpc.CallOption) (*proto.StatusResponse, error) {
	return nil, nil
}

func (stubClient) Ping(context.Context, *proto.Empty, ...grpc.CallOption) (*proto.StatusResponse, error) {
	return nil, nil
}

func (stubClient) Handshake(context.Context, *proto.HandshakeRequest, ...grpc.CallOption) (*proto.HandshakeResponse, error) {
	return nil, nil
}

func (stubClient) CreateSession(context.Context, *proto.SessionRequest, ...grpc.CallOption) (*proto.SessionResponse, error) {
	return nil, nil
}

func (stubClient) SendResumeBitmap(context.Context, ...grpc.CallOption) (proto.Replication_SendResumeBitmapClient, error) {
	return nil, nil
}

func (stubClient) SendFinalManifest(context.Context, *proto.ManifestMessage, ...grpc.CallOption) (*proto.StatusResponse, error) {
	return nil, nil
}

func (stubClient) Finalize(context.Context, *proto.FinalizeRequest, ...grpc.CallOption) (*proto.StatusResponse, error) {
	return nil, nil
}

func (stubClient) AckStream(context.Context, ...grpc.CallOption) (proto.Replication_AckStreamClient, error) {
	return nil, nil
}

func (stubClient) Probe(context.Context, *proto.ProbeRequest, ...grpc.CallOption) (*proto.StatusResponse, error) {
	return nil, nil
}

func (stubClient) StartSync(context.Context, *proto.StartSyncRequest, ...grpc.CallOption) (*proto.StatusResponse, error) {
	return nil, nil
}

func (stubClient) Cancel(context.Context, *proto.CancelRequest, ...grpc.CallOption) (*proto.StatusResponse, error) {
	return nil, nil
}

func (stubClient) ProgressStream(context.Context, *proto.ProgressRequest, ...grpc.CallOption) (proto.Replication_ProgressStreamClient, error) {
	return nil, nil
}

func (stubClient) BuildManifest(context.Context, *proto.BuildManifestRequest, ...grpc.CallOption) (*proto.StatusResponse, error) {
	return nil, nil
}

func (stubClient) Verify(context.Context, *proto.VerifyRequest, ...grpc.CallOption) (*proto.StatusResponse, error) {
	return nil, nil
}

func (fakeHandshakeClient) Handshake(context.Context, *proto.HandshakeRequest, ...grpc.CallOption) (*proto.HandshakeResponse, error) {
	return nil, errors.New("fail")
}

func TestHandshakeError(t *testing.T) {
	c := fakeHandshakeClient{}
	if _, err := Handshake(context.Background(), c, &proto.HandshakeRequest{}); err == nil {
		t.Fatalf("expected error")
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
