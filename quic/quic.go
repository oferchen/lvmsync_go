package quic

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"

	q "github.com/quic-go/quic-go"
)

type Config struct {
	TLSCert       string
	TLSKey        string
	CACert        string
	AllowInsecure bool
}

func NewTLSConfig(conf Config, server bool) (*tls.Config, error) {
	if conf.AllowInsecure {
		return &tls.Config{
			InsecureSkipVerify: true,
			MinVersion:         tls.VersionTLS13,
			MaxVersion:         tls.VersionTLS13,
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
	}
	if server {
		cfg.ClientAuth = tls.RequireAndVerifyClientCert
		cfg.ClientCAs = pool
	} else {
		cfg.RootCAs = pool
	}
	return cfg, nil
}

func NewQUICConfig() *q.Config {
	return &q.Config{Allow0RTT: false}
}
