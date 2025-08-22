package manifest

import (
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"

	"github.com/oferchen/lvmsync_go/internal/config"
	manifestpkg "github.com/oferchen/lvmsync_go/manifest"
)

func TestRunCheckHealthy(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "good.man")
	idx, err := manifestpkg.Create(path, "dev", 8192, 0, 0, 0, 4096, 0, 0, 0, 0)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := idx.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	if err := Run(cfg, []string{"check", path}, zap.NewNop()); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestRunCheckCorrupted(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.man")
	idx, err := manifestpkg.Create(path, "dev", 8192, 0, 0, 0, 4096, 0, 0, 0, 0)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := idx.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if err := os.Truncate(path, info.Size()-1); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	if err := Run(cfg, []string{"check", path}, zap.NewNop()); err == nil {
		t.Fatalf("expected error for corrupted manifest")
	}
}

func TestRunCheckMissingFile(t *testing.T) {
	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	if err := Run(cfg, []string{"check", "missing.man"}, zap.NewNop()); err == nil {
		t.Fatalf("expected error for missing manifest")
	}
}
