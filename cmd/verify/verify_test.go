package verify

import (
	"os"
	"testing"

	"go.uber.org/zap"
)

func writeTempFile(t *testing.T, data []byte) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "verify")
	if err != nil {
		t.Fatalf("create temp: %v", err)
	}
	if _, err := f.Write(data); err != nil {
		t.Fatalf("write temp: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close temp: %v", err)
	}
	return f.Name()
}

func TestVerifySuccess(t *testing.T) {
	data := make([]byte, 8192)
	src := writeTempFile(t, data)
	dst := writeTempFile(t, data)
	if err := Run([]string{"--block_size", "8K", src, dst}, zap.NewNop()); err != nil {
		t.Fatalf("verify success: %v", err)
	}
}

func TestVerifyMismatch(t *testing.T) {
	srcData := make([]byte, 8192)
	dstData := make([]byte, 8192)
	dstData[0] = 1
	src := writeTempFile(t, srcData)
	dst := writeTempFile(t, dstData)
	if err := Run([]string{"--block_size", "8K", src, dst}, zap.NewNop()); err == nil {
		t.Fatalf("expected mismatch error")
	}
}
