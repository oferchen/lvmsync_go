// Package verify contains tests for the verify command.
package verify

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/zap"

	"lvmsync_go/internal/config"
	manifestpkg "lvmsync_go/manifest"
)

func TestDryRunFlagOverridesEnvAndYAML(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfgFile, []byte("dry_run: true\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("LVMSYNC_DRY_RUN", "true")
	called := false
	rebuild := func(_ context.Context, _ string, _ string, _ *zap.Logger, _ time.Duration, _ bool, _ uint32, _ uint32, _ uint32, _ uint32, _ ...manifestpkg.IndexOption) error {
		return nil
	}
	verify := func(_ context.Context, _ *config.Config, _ string, _ string, _ string, _ *zap.Logger) error {
		called = true
		return nil
	}
	r := NewRunnerWithDeps(rebuild, nil, verify)
	if err := r.Run([]string{"--config", cfgFile, "--dry-run=false", "src", "dst"}, zap.NewNop()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !called {
		t.Fatalf("verifyDevices should be called when flag overrides env and config")
	}
}

func TestDryRunEnvOverridesYAML(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfgFile, []byte("dry_run: false\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("LVMSYNC_DRY_RUN", "true")
	called := false
	rebuild := func(_ context.Context, _ string, _ string, _ *zap.Logger, _ time.Duration, _ bool, _ uint32, _ uint32, _ uint32, _ uint32, _ ...manifestpkg.IndexOption) error {
		return nil
	}
	verify := func(_ context.Context, _ *config.Config, _ string, _ string, _ string, _ *zap.Logger) error {
		called = true
		return nil
	}
	r := NewRunnerWithDeps(rebuild, nil, verify)
	src := filepath.Join(dir, "src")
	if err := os.WriteFile(src, []byte("data"), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}
	if err := r.Run([]string{"--config", cfgFile, src, "dst"}, zap.NewNop()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if called {
		t.Fatalf("verifyDevices should not be called when env overrides config")
	}
}
