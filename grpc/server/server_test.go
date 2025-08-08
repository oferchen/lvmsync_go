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
	srv := New(cfg, agent)
	go func(t *testing.T) {
		if err := srv.Serve(lis); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			t.Errorf("srv.Serve: %v", err)
		}
	}(t)
	dialer := func(context.Context, string) (net.Conn, error) { return lis.Dial() }
	conn, err := grpc.DialContext(context.Background(), "bufnet", grpc.WithContextDialer(dialer), grpc.WithTransportCredentials(creds))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	cleanup := func() {
		conn.Close()
		srv.Stop()
	}
	return proto.NewReplicationClient(conn), cleanup
}

func ctxWithRole(role string) context.Context {
	return metadata.NewOutgoingContext(context.Background(), metadata.Pairs("role", role))
}

func TestAuthorizeInterceptor(t *testing.T) {
	client, cleanup := newClient(t, Config{AllowInsecure: true}, nil, insecure.NewCredentials())
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
	t.Run("success", func(t *testing.T) {
		agent := &mockAgent{}
		client, cleanup := newClient(t, Config{AllowInsecure: true}, agent, insecure.NewCredentials())
		defer cleanup()
		resp, err := client.LockVolume(ctxWithRole("replicator"), &proto.LockRequest{VolumeName: "vol", Requester: "req"})
		if err != nil {
			t.Fatalf("call failed: %v", err)
		}
		if !resp.GetOk() {
			t.Fatalf("expected ok response")
		}
	})
	t.Run("lock held", func(t *testing.T) {
		agent := &mockAgent{lock: func(ctx context.Context, v, r string) error { return errors.New("already locked") }}
		client, cleanup := newClient(t, Config{AllowInsecure: true}, agent, insecure.NewCredentials())
		defer cleanup()
		resp, err := client.LockVolume(ctxWithRole("replicator"), &proto.LockRequest{VolumeName: "vol", Requester: "req"})
		if err != nil {
			t.Fatalf("call failed: %v", err)
		}
		if resp.GetOk() || resp.GetMessage() != "already locked" {
			t.Fatalf("unexpected response %#v", resp)
		}
	})
	t.Run("no agent", func(t *testing.T) {
		client, cleanup := newClient(t, Config{AllowInsecure: true}, nil, insecure.NewCredentials())
		defer cleanup()
		resp, err := client.LockVolume(ctxWithRole("replicator"), &proto.LockRequest{VolumeName: "vol", Requester: "req"})
		if err != nil {
			t.Fatalf("call failed: %v", err)
		}
		if resp.GetOk() || resp.GetMessage() != "agent not configured" {
			t.Fatalf("unexpected response %#v", resp)
		}
	})
}

func TestGetVolumeMetadata(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		agent := &mockAgent{getMeta: func(ctx context.Context, v string) (lvmagent.VolumeMetadata, error) {
			return lvmagent.VolumeMetadata{VolumeName: v, SizeBytes: 1, ChunkSize: 2}, nil
		}}
		client, cleanup := newClient(t, Config{AllowInsecure: true}, agent, insecure.NewCredentials())
		defer cleanup()
		resp, err := client.GetVolumeMetadata(ctxWithRole("replicator"), &proto.LockRequest{VolumeName: "vol"})
		if err != nil {
			t.Fatalf("call failed: %v", err)
		}
		if resp.GetVolumeName() != "vol" || resp.GetSizeBytes() != 1 || resp.GetChunkSize() != 2 {
			t.Fatalf("unexpected response %#v", resp)
		}
	})
	t.Run("agent error", func(t *testing.T) {
		agent := &mockAgent{getMeta: func(ctx context.Context, v string) (lvmagent.VolumeMetadata, error) {
			return lvmagent.VolumeMetadata{}, errors.New("fail")
		}}
		client, cleanup := newClient(t, Config{AllowInsecure: true}, agent, insecure.NewCredentials())
		defer cleanup()
		_, err := client.GetVolumeMetadata(ctxWithRole("replicator"), &proto.LockRequest{VolumeName: "vol"})
		if status.Code(err) == codes.OK {
			t.Fatalf("expected error")
		}
	})
	t.Run("no agent", func(t *testing.T) {
		client, cleanup := newClient(t, Config{AllowInsecure: true}, nil, insecure.NewCredentials())
		defer cleanup()
		_, err := client.GetVolumeMetadata(ctxWithRole("replicator"), &proto.LockRequest{VolumeName: "vol"})
		if status.Code(err) != codes.FailedPrecondition {
			t.Fatalf("expected failed precondition, got %v", err)
		}
	})
}

func TestSendVolumeMetadata(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		agent := &mockAgent{}
		client, cleanup := newClient(t, Config{AllowInsecure: true}, agent, insecure.NewCredentials())
		defer cleanup()
		resp, err := client.SendVolumeMetadata(ctxWithRole("replicator"), &proto.VolumeMetadata{VolumeName: "vol"})
		if err != nil {
			t.Fatalf("call failed: %v", err)
		}
		if !resp.GetOk() {
			t.Fatalf("expected ok")
		}
	})
	t.Run("agent error", func(t *testing.T) {
		agent := &mockAgent{sendMeta: func(ctx context.Context, md lvmagent.VolumeMetadata) error { return errors.New("checksum mismatch") }}
		client, cleanup := newClient(t, Config{AllowInsecure: true}, agent, insecure.NewCredentials())
		defer cleanup()
		resp, err := client.SendVolumeMetadata(ctxWithRole("replicator"), &proto.VolumeMetadata{VolumeName: "vol"})
		if err != nil {
			t.Fatalf("call failed: %v", err)
		}
		if resp.GetOk() || resp.GetMessage() != "checksum mismatch" {
			t.Fatalf("unexpected response %#v", resp)
		}
	})
	t.Run("no agent", func(t *testing.T) {
		client, cleanup := newClient(t, Config{AllowInsecure: true}, nil, insecure.NewCredentials())
		defer cleanup()
		resp, err := client.SendVolumeMetadata(ctxWithRole("replicator"), &proto.VolumeMetadata{VolumeName: "vol"})
		if err != nil {
			t.Fatalf("call failed: %v", err)
		}
		if resp.GetOk() || resp.GetMessage() != "agent not configured" {
			t.Fatalf("unexpected response %#v", resp)
		}
	})
}

func TestStartTransferSession(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		agent := &mockAgent{}
		client, cleanup := newClient(t, Config{AllowInsecure: true}, agent, insecure.NewCredentials())
		defer cleanup()
		resp, err := client.StartTransferSession(ctxWithRole("replicator"), &proto.LockRequest{VolumeName: "vol", Requester: "req"})
		if err != nil {
			t.Fatalf("call failed: %v", err)
		}
		if !resp.GetOk() {
			t.Fatalf("expected ok")
		}
	})
	t.Run("agent error", func(t *testing.T) {
		agent := &mockAgent{startSess: func(ctx context.Context, v, r string) error { return errors.New("session failed") }}
		client, cleanup := newClient(t, Config{AllowInsecure: true}, agent, insecure.NewCredentials())
		defer cleanup()
		resp, err := client.StartTransferSession(ctxWithRole("replicator"), &proto.LockRequest{VolumeName: "vol", Requester: "req"})
		if err != nil {
			t.Fatalf("call failed: %v", err)
		}
		if resp.GetOk() || resp.GetMessage() != "session failed" {
			t.Fatalf("unexpected response %#v", resp)
		}
	})
	t.Run("no agent", func(t *testing.T) {
		client, cleanup := newClient(t, Config{AllowInsecure: true}, nil, insecure.NewCredentials())
		defer cleanup()
		resp, err := client.StartTransferSession(ctxWithRole("replicator"), &proto.LockRequest{VolumeName: "vol", Requester: "req"})
		if err != nil {
			t.Fatalf("call failed: %v", err)
		}
		if resp.GetOk() || resp.GetMessage() != "agent not configured" {
			t.Fatalf("unexpected response %#v", resp)
		}
	})
}

func TestFinalizeSync(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		agent := &mockAgent{finalize: func(ctx context.Context, v, r string) error { return nil }, unlock: func(ctx context.Context, v, r string) error { return nil }}
		client, cleanup := newClient(t, Config{AllowInsecure: true}, agent, insecure.NewCredentials())
		defer cleanup()
		resp, err := client.FinalizeSync(ctxWithRole("replicator"), &proto.LockRequest{VolumeName: "vol", Requester: "req"})
		if err != nil {
			t.Fatalf("call failed: %v", err)
		}
		if !resp.GetOk() {
			t.Fatalf("expected ok")
		}
	})
	t.Run("finalize error", func(t *testing.T) {
		agent := &mockAgent{finalize: func(ctx context.Context, v, r string) error { return errors.New("sync fail") }}
		client, cleanup := newClient(t, Config{AllowInsecure: true}, agent, insecure.NewCredentials())
		defer cleanup()
		resp, err := client.FinalizeSync(ctxWithRole("replicator"), &proto.LockRequest{VolumeName: "vol", Requester: "req"})
		if err != nil {
			t.Fatalf("call failed: %v", err)
		}
		if resp.GetOk() || resp.GetMessage() != "sync fail" {
			t.Fatalf("unexpected response %#v", resp)
		}
	})
	t.Run("unlock error", func(t *testing.T) {
		agent := &mockAgent{finalize: func(ctx context.Context, v, r string) error { return nil }, unlock: func(ctx context.Context, v, r string) error { return errors.New("unlock fail") }}
		client, cleanup := newClient(t, Config{AllowInsecure: true}, agent, insecure.NewCredentials())
		defer cleanup()
		resp, err := client.FinalizeSync(ctxWithRole("replicator"), &proto.LockRequest{VolumeName: "vol", Requester: "req"})
		if err != nil {
			t.Fatalf("call failed: %v", err)
		}
		if resp.GetOk() || resp.GetMessage() != "unlock fail" {
			t.Fatalf("unexpected response %#v", resp)
		}
	})
	t.Run("no agent", func(t *testing.T) {
		client, cleanup := newClient(t, Config{AllowInsecure: true}, nil, insecure.NewCredentials())
		defer cleanup()
		resp, err := client.FinalizeSync(ctxWithRole("replicator"), &proto.LockRequest{VolumeName: "vol", Requester: "req"})
		if err != nil {
			t.Fatalf("call failed: %v", err)
		}
		if resp.GetOk() || resp.GetMessage() != "agent not configured" {
			t.Fatalf("unexpected response %#v", resp)
		}
	})
}

func TestGetStatus(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		agent := &mockAgent{status: func(ctx context.Context, v, r string) (string, error) { return "ok", nil }}
		client, cleanup := newClient(t, Config{AllowInsecure: true}, agent, insecure.NewCredentials())
		defer cleanup()
		resp, err := client.GetStatus(ctxWithRole("replicator"), &proto.LockRequest{VolumeName: "vol", Requester: "req"})
		if err != nil {
			t.Fatalf("call failed: %v", err)
		}
		if !resp.GetOk() || resp.GetMessage() != "ok" {
			t.Fatalf("unexpected response %#v", resp)
		}
	})
	t.Run("agent error", func(t *testing.T) {
		agent := &mockAgent{status: func(ctx context.Context, v, r string) (string, error) { return "", errors.New("bad") }}
		client, cleanup := newClient(t, Config{AllowInsecure: true}, agent, insecure.NewCredentials())
		defer cleanup()
		resp, err := client.GetStatus(ctxWithRole("replicator"), &proto.LockRequest{VolumeName: "vol", Requester: "req"})
		if err != nil {
			t.Fatalf("call failed: %v", err)
		}
		if resp.GetOk() || resp.GetMessage() != "bad" {
			t.Fatalf("unexpected response %#v", resp)
		}
	})
	t.Run("no agent", func(t *testing.T) {
		client, cleanup := newClient(t, Config{AllowInsecure: true}, nil, insecure.NewCredentials())
		defer cleanup()
		resp, err := client.GetStatus(ctxWithRole("replicator"), &proto.LockRequest{VolumeName: "vol", Requester: "req"})
		if err != nil {
			t.Fatalf("call failed: %v", err)
		}
		if resp.GetOk() || resp.GetMessage() != "agent not configured" {
			t.Fatalf("unexpected response %#v", resp)
		}
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
	if err := os.WriteFile(srvCertFile, serverCertPEM, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(srvKeyFile, serverKeyPEM, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(caFile, caPEM, 0600); err != nil {
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
