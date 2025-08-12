package transport

import (
	"crypto/tls"
	"testing"

	"github.com/klauspost/cpuid/v2"
)

func TestDefaultTLSConfig(t *testing.T) {
	cfg := DefaultTLSConfig()
	if cfg.MinVersion != tls.VersionTLS13 {
		t.Fatalf("expected TLS1.3, got %v", cfg.MinVersion)
	}
    if cpuid.CPU.Supports(cpuid.AESNI) || cpuid.CPU.Supports(cpuid.AESARM) {
        if cfg.CipherSuites[0] != tls.TLS_AES_128_GCM_SHA256 {
            t.Fatalf("AES not preferred")
        }
    } else {
		if cfg.CipherSuites[0] != tls.TLS_CHACHA20_POLY1305_SHA256 {
			t.Fatalf("ChaCha not preferred")
		}
	}
}
