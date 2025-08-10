package server

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	lvmagent "lvmsync_go/internal/lvm"
	"lvmsync_go/proto"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

const bufSize = 1024 * 1024

// mockAgent implements lvmagent.Agent for testing.
type mockAgent struct {
	lock      func(ctx context.Context, volume, requester string) error
	unlock    func(ctx context.Context, volume, requester string) error
	getMeta   func(ctx context.Context, volume string) (lvmagent.VolumeMetadata, error)
	sendMeta  func(ctx context.Context, md lvmagent.VolumeMetadata) error
	startSess func(ctx context.Context, volume, requester string) error
	finalize  func(ctx context.Context, volume, requester string) error
	status    func(ctx context.Context, volume, requester string) (string, error)
}

func (m *mockAgent) Lock(ctx context.Context, volume, requester string) error {
	if m.lock != nil {
		return m.lock(ctx, volume, requester)
	}
	return nil
}
func (m *mockAgent) Unlock(ctx context.Context, volume, requester string) error {
	if m.unlock != nil {
		return m.unlock(ctx, volume, requester)
	}
	return nil
}
func (m *mockAgent) GetMetadata(ctx context.Context, volume string) (lvmagent.VolumeMetadata, error) {
	if m.getMeta != nil {
		return m.getMeta(ctx, volume)
	}
	return lvmagent.VolumeMetadata{}, nil
}
func (m *mockAgent) SendMetadata(ctx context.Context, md lvmagent.VolumeMetadata) error {
	if m.sendMeta != nil {
		return m.sendMeta(ctx, md)
	}
	return nil
}
func (m *mockAgent) StartTransferSession(ctx context.Context, volume, requester string) error {
	if m.startSess != nil {
		return m.startSess(ctx, volume, requester)
	}
	return nil
}
func (m *mockAgent) FinalizeSync(ctx context.Context, volume, requester string) error {
	if m.finalize != nil {
		return m.finalize(ctx, volume, requester)
	}
	return nil
}
func (m *mockAgent) GetStatus(ctx context.Context, volume, requester string) (string, error) {
	if m.status != nil {
		return m.status(ctx, volume, requester)
	}
	return "", nil
}

func newClient(t *testing.T, cfg Config, agent lvmagent.Agent, creds credentials.TransportCredentials) (proto.ReplicationClient, func()) {
	t.Helper()
	lis := bufconn.Listen(bufSize)
	srv, err := New(cfg, agent)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	go func(t *testing.T) {
		err := srv.Serve(lis)
		if err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			t.Errorf("srv.Serve: %v", err)
		}
	}(t)
	dialer := func(context.Context, string) (net.Conn, error) { return lis.Dial() }
	conn, err := grpc.NewClient("passthrough:///bufnet", grpc.WithContextDialer(dialer), grpc.WithTransportCredentials(creds))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	cleanup := func() {
		if err := conn.Close(); err != nil {
			t.Errorf("conn.Close: %v", err)
		}
		srv.Stop()
	}
	return proto.NewReplicationClient(conn), cleanup
}

func newInsecureClient(t *testing.T, agent lvmagent.Agent) (proto.ReplicationClient, func()) {
	return newClient(t, Config{AllowInsecure: true}, agent, insecure.NewCredentials())
}

func ctxWithRole(role string) context.Context {
	return metadata.NewOutgoingContext(context.Background(), metadata.Pairs("role", role))
}

// runStatusTest executes an RPC returning a StatusResponse and verifies ok/message fields.
func runStatusTest(t *testing.T, agent lvmagent.Agent, ok bool, msg string, call func(proto.ReplicationClient) (*proto.StatusResponse, error)) {
	t.Helper()
	client, cleanup := newInsecureClient(t, agent)
	defer cleanup()
	resp, err := call(client)
	if err != nil {
		t.Fatalf("call failed: %v", err)
	}
	if resp.GetOk() != ok || resp.GetMessage() != msg {
		t.Fatalf("unexpected response %#v", resp)
	}
}

type testCase struct {
	name  string
	agent lvmagent.Agent
	ok    bool
	msg   string
}

// runAgentTest executes an RPC for each test case and validates its StatusResponse.
func runAgentTest(t *testing.T, cases []testCase, call func(proto.ReplicationClient) (*proto.StatusResponse, error)) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runStatusTest(t, tc.agent, tc.ok, tc.msg, call)
		})
	}
}

func TestAuthorizeInterceptor(t *testing.T) {
	client, cleanup := newInsecureClient(t, nil)
	defer cleanup()

	if _, err := client.Ping(ctxWithRole("replicator"), &proto.Empty{}); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if _, err := client.Ping(ctxWithRole("other"), &proto.Empty{}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("expected permission denied, got %v", err)
	}
	if _, err := client.Ping(context.Background(), &proto.Empty{}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("expected permission denied, got %v", err)
	}
}

func TestLockVolume(t *testing.T) {
	cases := []testCase{
		{"success", &mockAgent{}, true, ""},
		{"lock held", &mockAgent{lock: func(_ context.Context, _, _ string) error { return errors.New("already locked") }}, false, "already locked"}, // parameters unused
		{"no agent", nil, false, "agent not configured"},
	}
	runAgentTest(t, cases, func(client proto.ReplicationClient) (*proto.StatusResponse, error) {
		return client.LockVolume(ctxWithRole("replicator"), &proto.LockRequest{VolumeName: "vol", Requester: "req"})
	})
}

//revive:disable-next-line:cognitive-complexity
func TestGetVolumeMetadata(t *testing.T) {
	tests := []struct {
		name    string
		agent   lvmagent.Agent
		wantErr bool
	}{
		{"success", &mockAgent{getMeta: func(_ context.Context, v string) (lvmagent.VolumeMetadata, error) {
			// ctx is unused.
			return lvmagent.VolumeMetadata{VolumeName: v, SizeBytes: 1, ChunkSize: 2}, nil
		}}, false},
		{"agent error", &mockAgent{getMeta: func(_ context.Context, _ string) (lvmagent.VolumeMetadata, error) {
			// parameters are unused in this mock.
			return lvmagent.VolumeMetadata{}, errors.New("fail")
		}}, true},
		{"no agent", nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, cleanup := newInsecureClient(t, tt.agent)
			defer cleanup()
			resp, err := client.GetVolumeMetadata(ctxWithRole("replicator"), &proto.LockRequest{VolumeName: "vol"})
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				if tt.agent == nil && status.Code(err) != codes.FailedPrecondition {
					t.Fatalf("expected failed precondition, got %v", err)
				}
			} else {
				if err != nil {
					t.Fatalf("call failed: %v", err)
				}
				if resp.GetVolumeName() != "vol" || resp.GetSizeBytes() != 1 || resp.GetChunkSize() != 2 {
					t.Fatalf("unexpected response %#v", resp)
				}
			}
		})
	}
}

func TestSendVolumeMetadata(t *testing.T) {
	cases := []testCase{
		{"success", &mockAgent{}, true, ""},
		{"agent error", &mockAgent{sendMeta: func(_ context.Context, _ lvmagent.VolumeMetadata) error { return errors.New("checksum mismatch") }}, false, "checksum mismatch"}, // parameters unused
		{"no agent", nil, false, "agent not configured"},
	}
	runAgentTest(t, cases, func(client proto.ReplicationClient) (*proto.StatusResponse, error) {
		return client.SendVolumeMetadata(ctxWithRole("replicator"), &proto.VolumeMetadata{VolumeName: "vol"})
	})
}

func TestStartTransferSession(t *testing.T) {
	cases := []testCase{
		{"success", &mockAgent{}, true, ""},
		{"agent error", &mockAgent{startSess: func(_ context.Context, _, _ string) error { return errors.New("session failed") }}, false, "session failed"}, // parameters unused
		{"no agent", nil, false, "agent not configured"},
	}
	runAgentTest(t, cases, func(client proto.ReplicationClient) (*proto.StatusResponse, error) {
		return client.StartTransferSession(ctxWithRole("replicator"), &proto.LockRequest{VolumeName: "vol", Requester: "req"})
	})
}

func TestFinalizeSync(t *testing.T) {
	cases := []testCase{
		{"success", &mockAgent{finalize: func(_ context.Context, _, _ string) error { return nil }, unlock: func(_ context.Context, _, _ string) error { return nil }}, true, ""},                                        // parameters unused
		{"finalize error", &mockAgent{finalize: func(_ context.Context, _, _ string) error { return errors.New("sync fail") }}, false, "sync fail"},                                                                      // parameters unused
		{"unlock error", &mockAgent{finalize: func(_ context.Context, _, _ string) error { return nil }, unlock: func(_ context.Context, _, _ string) error { return errors.New("unlock fail") }}, false, "unlock fail"}, // parameters unused
		{"no agent", nil, false, "agent not configured"},
	}
	runAgentTest(t, cases, func(client proto.ReplicationClient) (*proto.StatusResponse, error) {
		return client.FinalizeSync(ctxWithRole("replicator"), &proto.LockRequest{VolumeName: "vol", Requester: "req"})
	})
}

func TestGetStatus(t *testing.T) {
	cases := []testCase{
		{"success", &mockAgent{status: func(_ context.Context, _, _ string) (string, error) { return "ok", nil }}, true, "ok"},                   // parameters unused
		{"agent error", &mockAgent{status: func(_ context.Context, _, _ string) (string, error) { return "", errors.New("bad") }}, false, "bad"}, // parameters unused
		{"no agent", nil, false, "agent not configured"},
	}
	runAgentTest(t, cases, func(client proto.ReplicationClient) (*proto.StatusResponse, error) {
		return client.GetStatus(ctxWithRole("replicator"), &proto.LockRequest{VolumeName: "vol", Requester: "req"})
	})
}

func generateTLS(t *testing.T) (Config, *tls.Config, *tls.Config) {
	ca := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	caDER, err := x509.CreateCertificate(rand.Reader, ca, ca, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})

	serverCert := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "server"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"server"},
	}
	serverKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	serverDER, err := x509.CreateCertificate(rand.Reader, serverCert, ca, &serverKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	serverCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: serverDER})
	serverKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(serverKey)})

	clientCert := &x509.Certificate{
		SerialNumber: big.NewInt(3),
		Subject:      pkix.Name{CommonName: "client"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	clientKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	clientDER, err := x509.CreateCertificate(rand.Reader, clientCert, ca, &clientKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	clientCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: clientDER})
	clientKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(clientKey)})

	dir := t.TempDir()
	srvCertFile := filepath.Join(dir, "server.pem")
	srvKeyFile := filepath.Join(dir, "server.key")
	caFile := filepath.Join(dir, "ca.pem")
	err = os.WriteFile(srvCertFile, serverCertPEM, 0600)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(srvKeyFile, serverKeyPEM, 0600)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(caFile, caPEM, 0600)
	if err != nil {
		t.Fatal(err)
	}

	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(caPEM)
	clientTLSCert, err := tls.X509KeyPair(clientCertPEM, clientKeyPEM)
	if err != nil {
		t.Fatal(err)
	}
	good := &tls.Config{RootCAs: pool, Certificates: []tls.Certificate{clientTLSCert}, MinVersion: tls.VersionTLS13, ServerName: "server"}
	bad := &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS13, ServerName: "server"}

	cfg := Config{TLSCert: srvCertFile, TLSKey: srvKeyFile, CACert: caFile}
	return cfg, good, bad
}

func TestMTLSValidation(t *testing.T) {
	cfg, good, bad := generateTLS(t)

	t.Run("valid client cert", func(t *testing.T) {
		client, cleanup := newClient(t, cfg, nil, credentials.NewTLS(good))
		defer cleanup()
		if _, err := client.Ping(ctxWithRole("replicator"), &proto.Empty{}); err != nil {
			t.Fatalf("expected success, got %v", err)
		}
	})

	t.Run("missing client cert", func(t *testing.T) {
		client, cleanup := newClient(t, cfg, nil, credentials.NewTLS(bad))
		defer cleanup()
		if _, err := client.Ping(ctxWithRole("replicator"), &proto.Empty{}); err == nil {
			t.Fatalf("expected error")
		}
	})
}

func TestSessionFlow(t *testing.T) {
	client, cleanup := newInsecureClient(t, nil)
	defer cleanup()
	ctx := ctxWithRole("replicator")
	hsResp, err := client.Handshake(ctx, &proto.HandshakeRequest{SectorSize: 512, Alignment: 512, MaxConcurrency: 1})
	if err != nil || !hsResp.GetOk() {
		t.Fatalf("Handshake: %v", err)
	}
	sess, err := client.CreateSession(ctx, &proto.SessionRequest{VolumeName: "vol", DeviceUuid: "dev", ClientCert: dummyCert(t)})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if len(sess.GetPsk()) == 0 || len(sess.GetServerCert()) == 0 {
		t.Fatalf("expected psk and cert in session response")
	}
	bmp, err := client.SendResumeBitmap(ctx)
	if err != nil {
		t.Fatalf("SendResumeBitmap: %v", err)
	}
	if err := bmp.Send(&proto.ResumeBitmap{SessionId: sess.GetSessionId(), Bitmap: []byte{1}}); err != nil {
		t.Fatalf("bitmap send: %v", err)
	}
	if _, err := bmp.CloseAndRecv(); err != nil {
		t.Fatalf("bitmap close: %v", err)
	}
	if _, err := client.SendFinalManifest(ctx, &proto.ManifestMessage{SessionId: sess.GetSessionId(), Manifest: []byte("{}")}); err != nil {
		t.Fatalf("SendFinalManifest: %v", err)
	}
	if _, err := client.Finalize(ctx, &proto.FinalizeRequest{SessionId: sess.GetSessionId()}); err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	ack, err := client.AckStream(ctx)
	if err != nil {
		t.Fatalf("AckStream: %v", err)
	}
	if err := ack.Send(&proto.Ack{SessionId: sess.GetSessionId(), Ok: true, Message: "ping"}); err != nil {
		t.Fatalf("ack send: %v", err)
	}
	if _, err := ack.Recv(); err != nil {
		t.Fatalf("ack recv: %v", err)
	}
}

func TestHandshakeFailure(t *testing.T) {
	client, cleanup := newInsecureClient(t, nil)
	defer cleanup()
	ctx := ctxWithRole("replicator")
	if _, err := client.Handshake(ctx, &proto.HandshakeRequest{}); err == nil {
		t.Fatalf("expected handshake error")
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

func TestNewTLSFailures(t *testing.T) {
	t.Run("missing key pair", func(t *testing.T) {
		if _, err := New(Config{TLSCert: "nope", TLSKey: "nope", CACert: "nope"}, nil); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("missing CA", func(t *testing.T) {
		cfg, _, _ := generateTLS(t)
		cfg.CACert = "nope"
		if _, err := New(cfg, nil); err == nil {
			t.Fatalf("expected error")
		}
	})

	t.Run("invalid CA", func(t *testing.T) {
		cfg, _, _ := generateTLS(t)
		badCA := filepath.Join(t.TempDir(), "bad.pem")
		if err := os.WriteFile(badCA, []byte("invalid"), 0600); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		cfg.CACert = badCA
		if _, err := New(cfg, nil); err == nil {
			t.Fatalf("expected error")
		}
	})
}
