// Package verify contains tests for the verify command.
package verify

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"bou.ke/monkey"
	"go.uber.org/zap"

	"lvmsync_go/internal/config"
)

func TestDryRunFlagOverridesEnvAndYAML(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfgFile, []byte("dry_run: true\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("LVMSYNC_DRY_RUN", "true")
	r := newStubRunner()
	called := false
	patch := monkey.Patch((*Runner).verifyDevices, func(*Runner, context.Context, *config.Config, string, string, string, *zap.Logger) error {
		called = true
		return nil
	})
	defer patch.Unpatch()
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
	r := newStubRunner()
	called := false
	patch := monkey.Patch((*Runner).verifyDevices, func(*Runner, context.Context, *config.Config, string, string, string, *zap.Logger) error {
		called = true
		return nil
	})
	defer patch.Unpatch()
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
