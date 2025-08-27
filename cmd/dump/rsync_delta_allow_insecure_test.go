package dump

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/oferchen/lvmsync_go/internal/config"
	digestpkg "github.com/oferchen/lvmsync_go/internal/digest"
)

type discardWriteCloser struct{ io.Writer }

func (discardWriteCloser) Close() error { return nil }

func TestRunnerStreamRsyncDeltaAllowInsecure(t *testing.T) {
	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	cfg.Delta = "rsync"
	cfg.ChecksumAlgorithm = digestpkg.SHA256

	dir := t.TempDir()
	orig := filepath.Join(dir, "orig")
	snap := filepath.Join(dir, "snap")
	data := []byte("data")
	if err := os.WriteFile(orig, data, 0o600); err != nil {
		t.Fatalf("write origin: %v", err)
	}
	if err := os.WriteFile(snap, data, 0o600); err != nil {
		t.Fatalf("write snap: %v", err)
	}

	core1, logs1 := observer.New(zap.WarnLevel)
	r := NewRunner()
	if err := r.streamRsyncDelta(context.Background(), cfg, discardWriteCloser{io.Discard}, snap, orig, cfg.ChecksumAlgorithm, zap.New(core1)); err == nil {
		t.Fatalf("expected error without AllowInsecure")
	}
	if entries := logs1.FilterMessage("plaintext_connection").All(); len(entries) != 0 {
		t.Fatalf("unexpected plaintext warning")
	}

	cfg.AllowInsecure = true
	core2, logs2 := observer.New(zap.WarnLevel)
	if err := r.streamRsyncDelta(context.Background(), cfg, discardWriteCloser{io.Discard}, snap, orig, cfg.ChecksumAlgorithm, zap.New(core2)); err != nil {
		t.Fatalf("streamRsyncDelta: %v", err)
	}
	if entries := logs2.FilterMessage("plaintext_connection").All(); len(entries) == 0 {
		t.Fatalf("expected plaintext warning")
	}
}
