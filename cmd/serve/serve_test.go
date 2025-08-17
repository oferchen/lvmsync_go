package serve

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"go.uber.org/zap"

	"lvmsync_go/common"
	"lvmsync_go/transport"
)

func TestEnvVarPrecedence(t *testing.T) {
	v := viper.New()
	cmd := &cobra.Command{Use: "serve"}
	if err := bindFlags(cmd, v); err != nil {
		t.Fatalf("bindFlags: %v", err)
	}

	t.Setenv("LVMSYNC_SERVE_TRANSPORT", "envtransport")
	t.Setenv("LVMSYNC_SERVE_QUIC_LISTEN", "envaddr:1234")
	t.Setenv("LVMSYNC_SERVE_TLS_CERT", "envcert.pem")
	t.Setenv("LVMSYNC_SERVE_TLS_KEY", "envkey.pem")
	t.Setenv("LVMSYNC_SERVE_CA_CERT", "envca.pem")
	t.Setenv("LVMSYNC_SERVE_ALLOW_INSECURE", "false")

	args := []string{
		"--transport", "flagtransport",
		"--tls-cert", "flagcert.pem",
		"--allow-insecure",
	}
	if err := cmd.ParseFlags(args); err != nil {
		t.Fatalf("parse flags: %v", err)
	}

	got := Options{
		Transport:     v.GetString("transport"),
		QUICListen:    v.GetString("quic-listen"),
		TLSCert:       v.GetString("tls-cert"),
		TLSKey:        v.GetString("tls-key"),
		CACert:        v.GetString("ca-cert"),
		AllowInsecure: v.GetBool("allow-insecure"),
	}
	want := Options{
		Transport:     "flagtransport",
		QUICListen:    "envaddr:1234",
		TLSCert:       "flagcert.pem",
		TLSKey:        "envkey.pem",
		CACert:        "envca.pem",
		AllowInsecure: true,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v want %+v", got, want)
	}
}

type bindErrViper struct {
	*viper.Viper
}

func (b *bindErrViper) BindPFlags(_ *pflag.FlagSet) error { return errors.New("bind fail") }

func TestBindFlagsError(t *testing.T) {
	v := &bindErrViper{Viper: viper.New()}
	cmd := &cobra.Command{Use: "serve"}
	if err := bindFlags(cmd, v); err == nil || err.Error() != "bind fail" {
		t.Fatalf("expected bind fail, got %v", err)
	}
}

func TestStartServer(t *testing.T) {
	logger := zap.NewNop()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	opts := Options{Transport: "quic", QUICListen: "127.0.0.1:0", AllowInsecure: true}
	errCh := make(chan error, 1)
	go func() { errCh <- startServer(ctx, opts, logger) }()
	time.Sleep(50 * time.Millisecond)
	cancel()
	if err := <-errCh; !errors.Is(err, context.Canceled) {
		t.Fatalf("startServer: %v", err)
	}
}

func TestStartServerMissingTLSFiles(t *testing.T) {
	logger := zap.NewNop()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	opts := Options{Transport: "quic", QUICListen: "127.0.0.1:0"}
	err := startServer(ctx, opts, logger)
	if err == nil {
		t.Fatalf("expected error for missing certs")
	}
	want := "tls-cert, tls-key, and ca-cert are required unless --allow-insecure is set"
	if err.Error() != want {
		t.Fatalf("got %q want %q", err.Error(), want)
	}
}

func TestStartServerInvalidKeyPairPath(t *testing.T) {
	logger := zap.NewNop()
	ctx := context.Background()
	dir := t.TempDir()
	cert := filepath.Join(dir, "cert.pem")
	key := filepath.Join(dir, "key.pem")
	ca := filepath.Join(dir, "ca.pem")
	opts := Options{Transport: "quic", QUICListen: "127.0.0.1:0", TLSCert: cert, TLSKey: key, CACert: ca}
	err := startServer(ctx, opts, logger)
	if err == nil || !strings.Contains(err.Error(), "load TLS key pair") {
		t.Fatalf("got %v want error containing %q", err, "load TLS key pair")
	}
}

func TestStartServerInvalidCACert(t *testing.T) {
	logger := zap.NewNop()
	ctx := context.Background()

	t.Run("unreadable", func(t *testing.T) {
		cert, key := writeKeyPair(t)
		ca := filepath.Join(t.TempDir(), "missing.pem")
		opts := Options{Transport: "quic", QUICListen: "127.0.0.1:0", TLSCert: cert, TLSKey: key, CACert: ca}
		err := startServer(ctx, opts, logger)
		if err == nil || !strings.Contains(err.Error(), "read CA cert") {
			t.Fatalf("got %v want error containing %q", err, "read CA cert")
		}
	})

	t.Run("malformed", func(t *testing.T) {
		cert, key := writeKeyPair(t)
		ca := filepath.Join(t.TempDir(), "ca.pem")
		if err := os.WriteFile(ca, []byte("not a cert"), 0600); err != nil {
			t.Fatalf("write ca: %v", err)
		}
		opts := Options{Transport: "quic", QUICListen: "127.0.0.1:0", TLSCert: cert, TLSKey: key, CACert: ca}
		err := startServer(ctx, opts, logger)
		if err == nil || !strings.Contains(err.Error(), "invalid CA cert") {
			t.Fatalf("got %v want error containing %q", err, "invalid CA cert")
		}
	})
}

func TestStartServerUnknownTransport(t *testing.T) {
	logger := zap.NewNop()
	ctx := context.Background()
	opts := Options{Transport: "unknown", QUICListen: "127.0.0.1:0", AllowInsecure: true}
	err := startServer(ctx, opts, logger)
	want := "get transport: transport \"unknown\" not registered"
	if err == nil || err.Error() != want {
		t.Fatalf("got %v want %q", err, want)
	}
}

func TestRunAcceptFailure(t *testing.T) {
	const name = "acceptfail"
	acceptErr := errors.New("accept failed")
	if err := transport.Register(name, func(cfg transport.Config) (transport.Interface, error) {
		return &failTransport{err: acceptErr}, nil
	}); err != nil {
		t.Fatalf("register: %v", err)
	}
	logger := zap.NewNop()
	err := Run([]string{"--transport", name, "--allow-insecure"}, logger)
	if !errors.Is(err, acceptErr) {
		t.Fatalf("got %v want %v", err, acceptErr)
	}
}

func TestStartServerHandshakeSuccess(t *testing.T) {
	logger := zap.NewNop()
	addr := freeUDPAddr(t)
	cert, key := writeKeyPair(t)
	ctx, cancel := context.WithCancel(context.Background())
	opts := Options{Transport: "quic", QUICListen: addr, TLSCert: cert, TLSKey: key, CACert: cert}
	errCh := make(chan error, 1)
	go func() { errCh <- startServer(ctx, opts, logger) }()
	time.Sleep(50 * time.Millisecond)

	pem, _ := os.ReadFile(cert)
	roots := x509.NewCertPool()
	roots.AppendCertsFromPEM(pem)
	tlsCert, _ := tls.LoadX509KeyPair(cert, key)
	cfg := transport.Config{Logger: logger, Roots: roots, ClientCert: tlsCert}
	tr, err := transport.Get("quic", cfg)
	if err != nil {
		t.Fatalf("get transport: %v", err)
	}
	conn, err := tr.Dial(context.Background(), addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	if _, err := tr.Negotiate(context.Background(), conn, transport.Client, common.Handshake{}); err != nil {
		t.Fatalf("negotiate: %v", err)
	}

	cancel()
	if err := <-errCh; !errors.Is(err, context.Canceled) {
		t.Fatalf("startServer: %v", err)
	}
	buf := make([]byte, 1)
	if _, err := conn.Read(buf); err == nil {
		t.Fatalf("expected connection closed")
	}
	conn.Close()
}

func TestStartServerHandshakeFailure(t *testing.T) {
	logger := zap.NewNop()
	addr := freeUDPAddr(t)
	cert, key := writeKeyPair(t)
	ctx, cancel := context.WithCancel(context.Background())
	opts := Options{Transport: "quic", QUICListen: addr, TLSCert: cert, TLSKey: key, CACert: cert}
	errCh := make(chan error, 1)
	go func() { errCh <- startServer(ctx, opts, logger) }()
	time.Sleep(50 * time.Millisecond)

	pem, _ := os.ReadFile(cert)
	roots := x509.NewCertPool()
	roots.AppendCertsFromPEM(pem)
	tlsCert, _ := tls.LoadX509KeyPair(cert, key)
	cfg := transport.Config{Logger: logger, Roots: roots, ClientCert: tlsCert}
	tr, err := transport.Get("quic", cfg)
	if err != nil {
		t.Fatalf("get transport: %v", err)
	}
	conn, err := tr.Dial(context.Background(), addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	if _, err := tr.Negotiate(context.Background(), conn, transport.Client, common.Handshake{CRC32C: true}); err == nil {
		t.Fatalf("expected handshake failure")
	}
	buf := make([]byte, 1)
	if _, err := conn.Read(buf); err == nil {
		t.Fatalf("expected connection closed")
	}
	conn.Close()
	cancel()
	if err := <-errCh; !errors.Is(err, context.Canceled) {
		t.Fatalf("startServer: %v", err)
	}
}

type failTransport struct{ err error }

func (t *failTransport) Name() string { return "acceptfail" }

func (t *failTransport) Dial(ctx context.Context, addr string) (net.Conn, error) { return nil, nil }

func (t *failTransport) Listen(ctx context.Context, addr string) (net.Listener, error) {
	return &failListener{err: t.err}, nil
}

func (t *failTransport) Negotiate(ctx context.Context, conn net.Conn, role transport.Role, hs common.Handshake) (common.Handshake, error) {
	return hs, nil
}

type failListener struct{ err error }

func (l *failListener) Accept() (net.Conn, error) { return nil, l.err }

func (l *failListener) Close() error { return nil }

func (l *failListener) Addr() net.Addr { return &net.TCPAddr{} }

func freeUDPAddr(t *testing.T) string {
	t.Helper()
	l, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatalf("listen udp: %v", err)
	}
	addr := l.LocalAddr().String()
	l.Close()
	return addr
}

func writeKeyPair(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := x509.Certificate{SerialNumber: big.NewInt(1), NotBefore: time.Now(), NotAfter: time.Now().Add(time.Hour), IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	certOut := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyOut := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")
	if err := os.WriteFile(certPath, certOut, 0600); err != nil {
		t.Fatalf("write cert: %v", err)
	}
	if err := os.WriteFile(keyPath, keyOut, 0600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	return certPath, keyPath
}
