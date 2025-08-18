package verify

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/zeebo/blake3"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	manifestpkg "lvmsync_go/manifest"
)

func TestRunLogsMismatchBlock(t *testing.T) {
	dir := t.TempDir()
	src := dir + "/src"
	dst := dir + "/dst"
	if err := os.WriteFile(src, []byte("foo"), 0o600); err != nil {
		t.Fatalf("write src: %v", err)
	}
	if err := os.WriteFile(dst, []byte("bar"), 0o600); err != nil {
		t.Fatalf("write dst: %v", err)
	}
	core, logs := observer.New(zapcore.InfoLevel)
	logger := zap.New(core)
	r := NewRunnerWithDeps(func(ctx context.Context, device, output string, logger *zap.Logger, interval time.Duration, allow bool, cdcMin, cdcAvg, cdcMax, hybrid uint32, opts ...manifestpkg.IndexOption) error {
		idx, err := manifestpkg.Create(output, "dev", 3, 0, 4096, 0, 0, 0, 0)
		if err != nil {
			return err
		}
		digest := blake3.Sum256([]byte("foo"))
		if err := idx.Set(0, 3, 0, 0, digest); err != nil {
			return err
		}
		return idx.Close()
	})
	err := r.Run([]string{src, dst}, logger)
	if err == nil {
		t.Fatalf("expected error")
	}
	if logs.FilterMessage("digest_mismatch").Len() == 0 {
		t.Fatalf("expected digest_mismatch log, got %v", logs.All())
	}
}

func TestRunLogsMismatchBlockSHA256(t *testing.T) {
	dir := t.TempDir()
	src := dir + "/src"
	dst := dir + "/dst"
	if err := os.WriteFile(src, []byte("foo"), 0o600); err != nil {
		t.Fatalf("write src: %v", err)
	}
	if err := os.WriteFile(dst, []byte("bar"), 0o600); err != nil {
		t.Fatalf("write dst: %v", err)
	}
	core, logs := observer.New(zapcore.InfoLevel)
	logger := zap.New(core)
	r := NewRunnerWithDeps(func(ctx context.Context, device, output string, logger *zap.Logger, interval time.Duration, allow bool, cdcMin, cdcAvg, cdcMax, hybrid uint32, opts ...manifestpkg.IndexOption) error {
		idx, err := manifestpkg.Create(output, "dev", 3, 0, 4096, 0, 0, 0, 0)
		if err != nil {
			return err
		}
		digest := blake3.Sum256([]byte("foo"))
		if err := idx.Set(0, 3, 0, 0, digest); err != nil {
			return err
		}
		return idx.Close()
	})
	err := r.Run([]string{"--digest", "sha256", src, dst}, logger)
	if err == nil {
		t.Fatalf("expected error")
	}
	if logs.FilterMessage("digest_mismatch").Len() == 0 {
		t.Fatalf("expected digest_mismatch log, got %v", logs.All())
	}
}
