package remote

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"os"
	"path/filepath"
	"testing"

	remotetest "lvmsync_go/remote/testutil"

	"golang.org/x/crypto/ssh"
)

func TestLoadPrivateKey(t *testing.T) {
	keyFile := remotetest.CreateTempKey(t)
	if _, err := readPrivateKey(keyFile); err != nil {
		t.Fatalf("loadPrivateKey valid: %v", err)
	}
	if _, err := readPrivateKey(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatalf("expected error for missing key")
	}
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
