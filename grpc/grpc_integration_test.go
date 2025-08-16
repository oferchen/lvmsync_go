package grpc

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	grpcserver "lvmsync_go/grpc/server"
	lvmagent "lvmsync_go/internal/lvm"
	"lvmsync_go/proto"
)

// TestGRPCOperations runs a basic gRPC workflow against a real TCP listener.
// The test is skipped by default as it requires network and certificate setup.
func TestGRPCOperations(t *testing.T) {
	t.Skip("integration test")

	cfg, tlsConf := generateTLS(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	agent := &testAgent{}
	logger := zap.NewNop()
	srv, cleanup, err := grpcserver.New(cfg, agent, logger)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	go srv.Serve(ln)
	defer func() {
		srv.GracefulStop()
		ln.Close()
		cleanup()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := grpc.DialContext(ctx, ln.Addr().String(), grpc.WithTransportCredentials(credentials.NewTLS(tlsConf)))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	client := proto.NewReplicationClient(conn)

	if _, err := client.Probe(ctx, &proto.ProbeRequest{VolumeName: "vol"}); err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if _, err := client.StartSync(ctx, &proto.StartSyncRequest{VolumeName: "vol", Requester: "req"}); err != nil {
		t.Fatalf("StartSync: %v", err)
	}
	if _, err := client.Cancel(ctx, &proto.CancelRequest{SessionId: "s"}); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	ps, err := client.ProgressStream(ctx, &proto.ProgressRequest{SessionId: "s"})
	if err != nil {
		t.Fatalf("ProgressStream: %v", err)
	}
	if _, err := ps.Recv(); err != nil && err != io.EOF {
		t.Fatalf("progress recv: %v", err)
	}
	if _, err := client.BuildManifest(ctx, &proto.BuildManifestRequest{SessionId: "s"}); err != nil {
		t.Fatalf("BuildManifest: %v", err)
	}
	if _, err := client.Verify(ctx, &proto.VerifyRequest{SessionId: "s"}); err != nil {
		t.Fatalf("Verify: %v", err)
	}
}

// testAgent implements the required agent methods for integration tests.
type testAgent struct{}

func (testAgent) Lock(ctx context.Context, volume, requester string) error   { return nil }
func (testAgent) Unlock(ctx context.Context, volume, requester string) error { return nil }
func (testAgent) GetMetadata(ctx context.Context, volume string) (lvmagent.VolumeMetadata, error) {
	return lvmagent.VolumeMetadata{}, nil
}
func (testAgent) SendMetadata(ctx context.Context, md lvmagent.VolumeMetadata) error { return nil }
func (testAgent) StartTransferSession(ctx context.Context, volume, requester string) error {
	return nil
}
func (testAgent) FinalizeSync(ctx context.Context, volume, requester string) error { return nil }
func (testAgent) GetStatus(ctx context.Context, volume, requester string) (string, error) {
	return "", nil
}
func (testAgent) Probe(ctx context.Context, volume string) error     { return nil }
func (testAgent) Cancel(ctx context.Context, sessionID string) error { return nil }
func (testAgent) Progress(ctx context.Context, sessionID string) (<-chan *proto.Progress, error) {
	ch := make(chan *proto.Progress, 1)
	ch <- &proto.Progress{SessionId: sessionID}
	close(ch)
	return ch, nil
}
func (testAgent) BuildManifest(ctx context.Context, sessionID string) error { return nil }
func (testAgent) Verify(ctx context.Context, sessionID string) error        { return nil }

// generateTLS creates a CA, server cert, and client cert for mTLS testing.
func generateTLS(t *testing.T) (grpcserver.Config, *tls.Config) {
	t.Helper()
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
		Subject:      pkix.Name{CommonName: "client", OrganizationalUnit: []string{"replicator"}},
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
	tlsConf := &tls.Config{RootCAs: pool, Certificates: []tls.Certificate{clientTLSCert}, MinVersion: tls.VersionTLS13, ServerName: "server"}

	cfg := grpcserver.Config{TLSCert: srvCertFile, TLSKey: srvKeyFile, CACert: caFile}
	return cfg, tlsConf
}
