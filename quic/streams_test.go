package quic

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"os"
	"testing"
	"time"

	q "github.com/quic-go/quic-go"
)

// generateCert creates a self-signed certificate for tests.
func generateCert(t *testing.T) (certFile, keyFile, caFile string) {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})
	certFile = writeTemp(t, certPEM)
	keyFile = writeTemp(t, keyPEM)
	caFile = certFile
	return
}

func writeTemp(t *testing.T, b []byte) string {
	f, err := os.CreateTemp(t.TempDir(), "cert")
	if err != nil {
		t.Fatalf("tempfile: %v", err)
	}
	if _, err := f.Write(b); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return f.Name()
}

func TestQUICHandshake(t *testing.T) {
	cert, key, ca := generateCert(t)
	serverTLS, err := NewTLSConfig(Config{TLSCert: cert, TLSKey: key, CACert: ca}, true)
	if err != nil {
		t.Fatalf("server tls: %v", err)
	}
	clientTLS, err := NewTLSConfig(Config{TLSCert: cert, TLSKey: key, CACert: ca}, false)
	if err != nil {
		t.Fatalf("client tls: %v", err)
	}
	qc := NewQUICConfig(128 * 1024)
	listener, err := q.ListenAddr("127.0.0.1:0", serverTLS, qc)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	expected := Negotiation{Protocol: "p", Algorithm: "a"}
	errCh := make(chan error, 1)
	go func() {
		conn, err := listener.Accept(context.Background())
		if err != nil {
			errCh <- err
			return
		}
		stream, err := conn.AcceptStream(context.Background())
		if err != nil {
			errCh <- err
			return
		}
		errCh <- Negotiate(stream, expected)
	}()

	conn, err := q.DialAddr(context.Background(), listener.Addr().String(), clientTLS, qc)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	stream, err := conn.OpenStreamSync(context.Background())
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	if err := WriteNegotiation(stream, expected); err != nil {
		t.Fatalf("write negotiation: %v", err)
	}
	if _, err := ReadNegotiation(bufio.NewReader(stream)); err != nil {
		t.Fatalf("read negotiation: %v", err)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("negotiate: %v", err)
	}
}

func TestQUICChunkTransfer(t *testing.T) {
	cert, key, ca := generateCert(t)
	serverTLS, err := NewTLSConfig(Config{TLSCert: cert, TLSKey: key, CACert: ca}, true)
	if err != nil {
		t.Fatalf("server tls: %v", err)
	}
	clientTLS, err := NewTLSConfig(Config{TLSCert: cert, TLSKey: key, CACert: ca}, false)
	if err != nil {
		t.Fatalf("client tls: %v", err)
	}
	qc := NewQUICConfig(256 * 1024)
	listener, err := q.ListenAddr("127.0.0.1:0", serverTLS, qc)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	expected := Negotiation{Protocol: "p"}
	done := make(chan error, 1)
	go func() {
		conn, err := listener.Accept(context.Background())
		if err != nil {
			done <- err
			return
		}
		hs, err := conn.AcceptStream(context.Background())
		if err != nil {
			done <- err
			return
		}
		if err := Negotiate(hs, expected); err != nil {
			done <- err
			return
		}
		ctrl, err := conn.AcceptUniStream(context.Background())
		if err != nil {
			done <- err
			return
		}
		manifest, err := io.ReadAll(ctrl)
		if err != nil {
			done <- err
			return
		}
		if string(manifest) != "manifest" {
			done <- fmt.Errorf("bad manifest: %s", manifest)
			return
		}
		data1, err := conn.AcceptStream(context.Background())
		if err != nil {
			done <- err
			return
		}
		b1, err := io.ReadAll(data1)
		if err != nil {
			done <- err
			return
		}
		data2, err := conn.AcceptStream(context.Background())
		if err != nil {
			done <- err
			return
		}
		b2, err := io.ReadAll(data2)
		if err != nil {
			done <- err
			return
		}
		if string(b1) != "chunk1" || string(b2) != "chunk2" {
			done <- fmt.Errorf("unexpected chunks")
			return
		}
		done <- nil
	}()

	conn, err := q.DialAddr(context.Background(), listener.Addr().String(), clientTLS, qc)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	hs, err := conn.OpenStreamSync(context.Background())
	if err != nil {
		t.Fatalf("handshake stream: %v", err)
	}
	if err := WriteNegotiation(hs, expected); err != nil {
		t.Fatalf("write negotiation: %v", err)
	}
	if _, err := ReadNegotiation(bufio.NewReader(hs)); err != nil {
		t.Fatalf("read negotiation: %v", err)
	}
	ss, err := OpenStreams(context.Background(), conn, 2)
	if err != nil {
		t.Fatalf("open streams: %v", err)
	}
	if _, err := ss.Control.Write([]byte("manifest")); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	_ = ss.Control.Close()
	if _, err := ss.Data[0].Write([]byte("chunk1")); err != nil {
		t.Fatalf("write chunk1: %v", err)
	}
	_ = ss.Data[0].Close()
	if _, err := ss.Data[1].Write([]byte("chunk2")); err != nil {
		t.Fatalf("write chunk2: %v", err)
	}
	_ = ss.Data[1].Close()
	if err := <-done; err != nil {
		t.Fatalf("server: %v", err)
	}
}
