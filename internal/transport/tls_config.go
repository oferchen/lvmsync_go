package transport

import (
	"crypto/tls"

	"github.com/klauspost/cpuid/v2"
)

// DefaultTLSConfig returns a TLS 1.3 configuration choosing the cipher
// priority based on CPU features. AES-GCM is preferred when AES hardware
// acceleration is available; otherwise ChaCha20-Poly1305 comes first.
func DefaultTLSConfig() *tls.Config {
	cfg := &tls.Config{MinVersion: tls.VersionTLS13}
	if cpuid.CPU.Supports(cpuid.AESNI) || cpuid.CPU.Supports(cpuid.AESARM) {
		cfg.CipherSuites = []uint16{tls.TLS_AES_128_GCM_SHA256, tls.TLS_CHACHA20_POLY1305_SHA256}
	} else {
		cfg.CipherSuites = []uint16{tls.TLS_CHACHA20_POLY1305_SHA256, tls.TLS_AES_128_GCM_SHA256}
	}
	return cfg
}
