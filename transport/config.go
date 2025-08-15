package transport

import (
	"crypto/tls"
	"crypto/x509"

	"go.uber.org/zap"
)

// Config carries shared transport configuration such as TLS roots,
// client certificates, logger, and security policy.
// Fields may be nil/zero when not applicable to a transport.
type Config struct {
	Roots         *x509.CertPool
	ClientCert    tls.Certificate
	ServerCert    tls.Certificate
	Logger        *zap.Logger
	AllowInsecure bool
	SSHUser       string
	SSHPassword   string
	SSHKnownHosts string
	SSHHostKey    string
	SSHKeyPath    string
	SSHUseAgent   bool
}
