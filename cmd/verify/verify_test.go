package verify

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"lvmsync_go/device"
	"lvmsync_go/manifest"
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

func TestVerifyDryRun(t *testing.T) {
	srcData := []byte{1}
	dstData := []byte{2}
	src := writeTempFile(t, srcData)
	dst := writeTempFile(t, dstData)
	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	if err := Run([]string{"--dry-run", src, dst}, logger); err != nil {
		t.Fatalf("dry run verify: %v", err)
	}
	entries := logs.All()
	found := false
	for _, e := range entries {
		if e.Message == "dry run" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected dry run log entry")
	}
}

func TestVerifyManifestMismatch(t *testing.T) {
	srcData := make([]byte, 4096)
	srcData[0] = 1
	dstData := make([]byte, 4096)
	src := writeTempFile(t, srcData)
	dst := writeTempFile(t, dstData)
	manPath := filepath.Join(t.TempDir(), "dst.man")
	prevUUID := device.SetUUIDFunc(func(context.Context, string) (string, error) { return "uuid-test", nil })
	defer device.SetUUIDFunc(prevUUID)
	if err := manifest.Rebuild(dst, manPath, zap.NewNop(), 0); err != nil {
		t.Fatalf("rebuild manifest: %v", err)
	}
	core, observed := observer.New(zap.ErrorLevel)
	logger := zap.New(core)
	if err := Run([]string{"--manifest_path", manPath, src, dst}, logger); err == nil {
		t.Fatalf("expected mismatch error")
	}
	logs := observed.All()
	if len(logs) != 1 {
		t.Fatalf("expected one log entry, got %d", len(logs))
	}
	log := logs[0]
	if log.Message != "digest_mismatch" {
		t.Fatalf("unexpected log message %q", log.Message)
	}
	if _, ok := log.ContextMap()["offset_bytes"]; !ok {
		t.Fatalf("missing offset_bytes field")
	}
	exp, ok := log.ContextMap()["expected_digest"].(string)
	if !ok {
		t.Fatalf("missing expected_digest field")
	}
	act, ok := log.ContextMap()["actual_digest"].(string)
	if !ok {
		t.Fatalf("missing actual_digest field")
	}
	if exp == act {
		t.Fatalf("expected and actual digests should differ")
	}
}

func TestVerifyManifestSuccess(t *testing.T) {
	data := make([]byte, 4096)
	src := writeTempFile(t, data)
	manPath := filepath.Join(t.TempDir(), "src.man")
	prevUUID := device.SetUUIDFunc(func(context.Context, string) (string, error) { return "uuid-test", nil })
	defer device.SetUUIDFunc(prevUUID)
	if err := manifest.Rebuild(src, manPath, zap.NewNop(), 0); err != nil {
		t.Fatalf("rebuild manifest: %v", err)
	}
	if err := Run([]string{"--manifest_path", manPath, src, src}, zap.NewNop()); err != nil {
		t.Fatalf("verify manifest success: %v", err)
	}
}
