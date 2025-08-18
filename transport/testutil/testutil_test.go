package testutil

import (
	"crypto/x509"
	"testing"
	"time"

	"lvmsync_go/common"
	_ "lvmsync_go/transport/h2"
	_ "lvmsync_go/transport/quic"
	_ "lvmsync_go/transport/ssh"
	_ "lvmsync_go/transport/tcp_tls"
)

func TestGenerateSelfSignedCert(t *testing.T) {
	cert, _ := GenerateSelfSignedCert(t)
	parsed, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatalf("parse cert: %v", err)
	}
	dur := parsed.NotAfter.Sub(parsed.NotBefore)
	if dur < time.Hour || dur > time.Hour+time.Second {
		t.Fatalf("unexpected validity %s", dur)
	}
}

func TestNewTransport(t *testing.T) {
	names := []string{"ssh", "tcp+tls", "h2", "quic"}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			tr := NewTransport(t, name)
			if tr.Name() != name {
				t.Fatalf("expected %s, got %s", name, tr.Name())
			}
		})
	}
}

func TestRunNegotiation(t *testing.T) {
	names := []string{"ssh", "tcp+tls", "h2", "quic"}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			tr := NewTransport(t, name)
			hs := common.Handshake{ALPN: "lvmsync", TLSVersion: "1.3", BlockSize: 4096}
			if name == "h2" {
				hs.ALPN = "h2"
			}
			srvRes, cliRes := RunNegotiation(t, tr, hs, hs)
			if srvRes.Err != nil {
				t.Fatalf("server negotiation: %v", srvRes.Err)
			}
			if cliRes.Err != nil {
				t.Fatalf("client negotiation: %v", cliRes.Err)
			}
			if srvRes.Peer.BlockSize != hs.BlockSize {
				t.Fatalf("server peer block size = %d want %d", srvRes.Peer.BlockSize, hs.BlockSize)
			}
			if cliRes.Peer.BlockSize != hs.BlockSize {
				t.Fatalf("client peer block size = %d want %d", cliRes.Peer.BlockSize, hs.BlockSize)
			}
		})
	}
}
