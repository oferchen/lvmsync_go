package quic

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"math"
	"os"

	"golang.org/x/sys/cpu"

	q "github.com/quic-go/quic-go"
)

type Config struct {
	TLSCert       string
	TLSKey        string
	CACert        string
	AllowInsecure bool
}

// cipherSuites selects AES-GCM when hardware acceleration is available and
// falls back to ChaCha20 otherwise.
func cipherSuites() []uint16 {
	aes := cpu.X86.HasAES || cpu.ARM64.HasAES
	if aes {
		return []uint16{
			tls.TLS_AES_128_GCM_SHA256,
			tls.TLS_CHACHA20_POLY1305_SHA256,
		}
	}
	return []uint16{
		tls.TLS_CHACHA20_POLY1305_SHA256,
		tls.TLS_AES_128_GCM_SHA256,
	}
}

func NewTLSConfig(conf Config, server bool) (*tls.Config, error) {
	if conf.AllowInsecure {
		return &tls.Config{
			InsecureSkipVerify: true,
			MinVersion:         tls.VersionTLS13,
			MaxVersion:         tls.VersionTLS13,
			NextProtos:         []string{"lvmsync"},
			CipherSuites:       cipherSuites(),
		}, nil
	}
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
	cfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS13,
		MaxVersion:   tls.VersionTLS13,
		NextProtos:   []string{"lvmsync"},
		CipherSuites: cipherSuites(),
	}
	if server {
		cfg.ClientAuth = tls.RequireAndVerifyClientCert
		cfg.ClientCAs = pool
	} else {
		cfg.RootCAs = pool
	}
	return cfg, nil
}

// NewQUICConfig returns a quic-go configuration tuned for the provided BDP
// (in bytes). It configures flow control windows and stream counts to keep
// roughly one BDP of data in flight.
func NewQUICConfig(bdpBytes int64) *q.Config {
	cfg := &q.Config{Allow0RTT: false}
	if bdpBytes <= 0 {
		return cfg
	}
	streams := int64(math.Max(1, float64(bdpBytes)/(64*1024)))
	window := uint64(bdpBytes * 2)
	cfg.MaxIncomingStreams = streams
	cfg.MaxIncomingUniStreams = 1
	cfg.MaxStreamReceiveWindow = window
	cfg.MaxConnectionReceiveWindow = window * uint64(streams)
	return cfg
}
