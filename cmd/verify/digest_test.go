package verify

import (
	"crypto/sha256"
	"testing"

	"github.com/zeebo/blake3"

	"github.com/oferchen/lvmsync_go/internal/config"
)

func TestDigestFuncValidAndInvalid(t *testing.T) {
	cfg := &config.Config{ChecksumAlgorithm: "sha256"}
	h, err := digestFunc(cfg)
	if err != nil {
		t.Fatalf("sha256: %v", err)
	}
	if h([]byte("data")) != sha256.Sum256([]byte("data")) {
		t.Fatalf("sha256 digest mismatch")
	}

	cfg.ChecksumAlgorithm = "blake3"
	h, err = digestFunc(cfg)
	if err != nil {
		t.Fatalf("blake3: %v", err)
	}
	if h([]byte("data")) != blake3.Sum256([]byte("data")) {
		t.Fatalf("blake3 digest mismatch")
	}

	cfg.ChecksumAlgorithm = "md5"
	if _, err := digestFunc(cfg); err == nil {
		t.Fatalf("expected error for unsupported algorithm")
	}
}
