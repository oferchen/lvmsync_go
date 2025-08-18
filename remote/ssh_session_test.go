package remote

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"os"
	"path/filepath"
	"strings"
	"testing"

	remotetest "lvmsync_go/remote/testutil"

	"golang.org/x/crypto/ssh"
)

func TestReadPrivateKey(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		keyFile := remotetest.CreateTempKey(t)
		if _, err := readPrivateKey(keyFile); err != nil {
			t.Fatalf("readPrivateKey valid: %v", err)
		}
	})

	t.Run("invalid_mode", func(t *testing.T) {
		keyFile := remotetest.CreateTempKey(t)
		if err := os.Chmod(keyFile, 0o644); err != nil {
			t.Fatalf("chmod: %v", err)
		}
		if _, err := readPrivateKey(keyFile); err == nil {
			t.Fatalf("expected error for open permissions")
		} else if !strings.Contains(err.Error(), "too open") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("parse_error", func(t *testing.T) {
		f, err := os.CreateTemp(t.TempDir(), "badkey")
		if err != nil {
			t.Fatalf("temp file: %v", err)
		}
		if _, err := f.Write([]byte("not a key")); err != nil {
			t.Fatalf("write: %v", err)
		}
		if err := f.Chmod(0o600); err != nil {
			t.Fatalf("chmod: %v", err)
		}
		if err := f.Close(); err != nil {
			t.Fatalf("close: %v", err)
		}
		if _, err := readPrivateKey(f.Name()); err == nil {
			t.Fatalf("expected parse error")
		}
	})
}

func TestLoadHostPublicKey(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	pub, err := ssh.NewPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatalf("NewPublicKey: %v", err)
	}
	file := filepath.Join(t.TempDir(), "host_key.pub")
	if err := os.WriteFile(file, pub.Marshal(), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	loaded, err := readHostPublicKey(file)
	if err != nil {
		t.Fatalf("loadHostPublicKey valid: %v", err)
	}
	if !bytes.Equal(loaded.Marshal(), pub.Marshal()) {
		t.Fatalf("loaded key mismatch")
	}
	if _, err := readHostPublicKey(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatalf("expected error for missing host key")
	}
}
