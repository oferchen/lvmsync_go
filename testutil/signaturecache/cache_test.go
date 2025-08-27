//go:build rsync

package signaturecache

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func randDigest(t *testing.T) []byte {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("rand.Read: %v", err)
	}
	return b
}

func TestEviction(t *testing.T) {
	dir := t.TempDir()
	c := New(dir, time.Hour, 2)
	d1 := randDigest(t)
	d2 := randDigest(t)
	d3 := randDigest(t)
	if err := c.Put("vg", "lv1", 1, d1); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := c.Put("vg", "lv2", 1, d2); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := c.Put("vg", "lv3", 1, d3); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if _, ok := c.Get("vg", "lv1", 1); ok {
		t.Fatalf("expected lv1 to be evicted")
	}
	if _, err := os.Stat(filepath.Join(dir, "vg", "lv1.sig")); !os.IsNotExist(err) {
		t.Fatalf("expected lv1.sig removed, got %v", err)
	}
}

func TestTTLExpiration(t *testing.T) {
	dir := t.TempDir()
	c := New(dir, 10*time.Millisecond, 10)
	d := randDigest(t)
	if err := c.Put("vg", "lv", 1, d); err != nil {
		t.Fatalf("Put: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	if _, ok := c.Get("vg", "lv", 1); ok {
		t.Fatalf("expected cache miss after TTL")
	}
	if _, err := os.Stat(filepath.Join(dir, "vg", "lv.sig")); !os.IsNotExist(err) {
		t.Fatalf("expected file removed after TTL, got %v", err)
	}
}

func TestDigestMismatchInvalidates(t *testing.T) {
	dir := t.TempDir()
	c := New(dir, time.Hour, 10)
	d1 := randDigest(t)
	d2 := randDigest(t)
	if err := c.Put("vg", "lv", 1, d1); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if ok := c.Check("vg", "lv", 1, d2); ok {
		t.Fatalf("expected mismatch")
	}
	if _, ok := c.Get("vg", "lv", 1); ok {
		t.Fatalf("expected entry removed after mismatch")
	}
}

func TestSizeMismatchInvalidates(t *testing.T) {
	dir := t.TempDir()
	c := New(dir, time.Hour, 10)
	d := randDigest(t)
	if err := c.Put("vg", "lv", 1, d); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if _, ok := c.Get("vg", "lv", 2); ok {
		t.Fatalf("expected miss on size mismatch")
	}
	if _, err := os.Stat(filepath.Join(dir, "vg", "lv.sig")); !os.IsNotExist(err) {
		t.Fatalf("expected file removed on size mismatch, got %v", err)
	}
}

func TestGetAndCheckFromFile(t *testing.T) {
	dir := t.TempDir()
	c := New(dir, time.Hour, 2)
	d := randDigest(t)
	// write file manually
	path := filepath.Join(dir, "vg", "lv.sig")
	fe := fileEntry{Size: 1, DigestHex: hex.EncodeToString(d), Timestamp: time.Now()}
	data, err := json.Marshal(fe)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, ok := c.Get("vg", "lv", 1); !ok {
		t.Fatalf("expected load from file")
	}
	if ok := c.Check("vg", "lv", 1, d); !ok {
		t.Fatalf("expected check pass")
	}
}
