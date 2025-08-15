package lvmsync

import (
	"os"
	"testing"

	verifycmd "lvmsync_go/cmd/verify"
	"lvmsync_go/config"
	"lvmsync_go/manifest"

	"github.com/zeebo/blake3"
	"github.com/zeebo/xxh3"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

type syncTrackerCore struct {
	zapcore.Core
	syncs *int
}

func (s syncTrackerCore) Sync() error {
	*s.syncs++
	return s.Core.Sync()
}

func TestRunCommandExecutes(t *testing.T) {
	t.Setenv("LVMSYNC_TRANSPORT_TRANSPORT", "ssh")
	var gotSrc, gotDst string
	var gotOpts RunOptions
	runCommand = func(src, dst string, opts RunOptions, logger *zap.Logger) error {
		gotSrc, gotDst, gotOpts = src, dst, opts
		return nil
	}
	t.Cleanup(func() { runCommand = func(src, dst string, opts RunOptions, logger *zap.Logger) error { return nil } })

	if err := Execute([]string{"run", "src", "dst"}, zap.NewNop()); err != nil {
		t.Fatalf("execute run: %v", err)
	}
	if gotSrc != "src" || gotDst != "dst" {
		t.Fatalf("unexpected args %q %q", gotSrc, gotDst)
	}
	if gotOpts.DryRun {
		t.Fatalf("expected dry-run false")
	}
	if gotOpts.Transport != "ssh" {
		t.Fatalf("unexpected transport %q", gotOpts.Transport)
	}
}

func TestRunCommandDryRun(t *testing.T) {
	src := t.TempDir() + "/src"
	if err := os.WriteFile(src, []byte("data"), 0o600); err != nil {
		t.Fatalf("write src: %v", err)
	}
	called := false
	runCommand = func(src, dst string, opts RunOptions, logger *zap.Logger) error {
		called = true
		return nil
	}
	t.Cleanup(func() { runCommand = func(src, dst string, opts RunOptions, logger *zap.Logger) error { return nil } })

	if err := Execute([]string{"run", "--dry-run", src, "dst"}, zap.NewNop()); err != nil {
		t.Fatalf("execute run dry-run: %v", err)
	}
	if called {
		t.Fatalf("runCommand should not be called in dry-run")
	}
}

func TestRunCommandDryRunEnv(t *testing.T) {
	src := t.TempDir() + "/src"
	if err := os.WriteFile(src, []byte("data"), 0o600); err != nil {
		t.Fatalf("write src: %v", err)
	}
	t.Setenv("LVMSYNC_DRY_RUN", "true")
	called := false
	runCommand = func(src, dst string, o RunOptions, logger *zap.Logger) error {
		called = true
		return nil
	}
	t.Cleanup(func() { runCommand = func(src, dst string, opts RunOptions, logger *zap.Logger) error { return nil } })

	if err := Execute([]string{"run", src, "dst"}, zap.NewNop()); err != nil {
		t.Fatalf("execute run with env: %v", err)
	}
	if called {
		t.Fatalf("runCommand should not be called when dry-run env set")
	}
}

func TestRunCommandInvalidConfig(t *testing.T) {
	called := false
	runCommand = func(src, dst string, opts RunOptions, logger *zap.Logger) error {
		called = true
		return nil
	}
	t.Cleanup(func() { runCommand = func(src, dst string, opts RunOptions, logger *zap.Logger) error { return nil } })

	if err := Execute([]string{"run", "--ssh_keepalive=0s", "src", "dst"}, zap.NewNop()); err == nil {
		t.Fatalf("expected error for invalid config")
	}
	if called {
		t.Fatalf("runCommand should not be called on invalid config")
	}
}

func TestManifestRebuildRoutes(t *testing.T) {
	var gotDevice string
	var dry bool
	manifestRebuild = func(device string, d bool, logger *zap.Logger) error {
		gotDevice, dry = device, d
		return nil
	}
	t.Cleanup(func() { manifestRebuild = func(device string, dryRun bool, logger *zap.Logger) error { return nil } })

	if err := Execute([]string{"manifest", "rebuild", "--dry-run", "/dev/vg0"}, zap.NewNop()); err != nil {
		t.Fatalf("execute rebuild: %v", err)
	}
	if gotDevice != "/dev/vg0" {
		t.Fatalf("unexpected device %q", gotDevice)
	}
	if !dry {
		t.Fatalf("expected dry-run true")
	}
}

func TestManifestRebuildInvalidConfig(t *testing.T) {
	called := false
	manifestRebuild = func(device string, dryRun bool, logger *zap.Logger) error {
		called = true
		return nil
	}
	t.Cleanup(func() { manifestRebuild = func(device string, dryRun bool, logger *zap.Logger) error { return nil } })

	if err := Execute([]string{"manifest", "rebuild", "--ssh_keepalive=0s", "/dev/vg0"}, zap.NewNop()); err == nil {
		t.Fatalf("expected error for invalid config")
	}
	if called {
		t.Fatalf("manifestRebuild should not be called on invalid config")
	}
}

func TestVerifyRoutes(t *testing.T) {
	var got []string
	verifyRun = func(a []string, logger *zap.Logger) error {
		got = append([]string{}, a...)
		return nil
	}
	t.Cleanup(func() {
		verifyRun = func(args []string, logger *zap.Logger) error { return verifycmd.Run(args, logger) }
	})

	if err := Execute([]string{"verify", "a", "b"}, zap.NewNop()); err != nil {
		t.Fatalf("execute verify: %v", err)
	}
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("unexpected args %v", got)
	}
}

func TestExecuteSyncsLogger(t *testing.T) {
	t.Setenv("LVMSYNC_TRANSPORT_TRANSPORT", "ssh")
	src := t.TempDir() + "/src"
	if err := os.WriteFile(src, []byte("data"), 0o600); err != nil {
		t.Fatalf("write src: %v", err)
	}
	runCommand = func(src, dst string, opts RunOptions, logger *zap.Logger) error { return nil }
	t.Cleanup(func() { runCommand = func(src, dst string, opts RunOptions, logger *zap.Logger) error { return nil } })
	var syncs int
	core, _ := observer.New(zap.InfoLevel)
	logger := zap.New(syncTrackerCore{Core: core, syncs: &syncs})
	if err := Execute([]string{"run", src, "dst"}, logger); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if syncs != 1 {
		t.Fatalf("expected logger.Sync called once, got %d", syncs)
	}
}

func TestEstimateTransferWithManifest(t *testing.T) {
	dir := t.TempDir()
	src := dir + "/src"
	// two 4-byte blocks; second block differs from manifest
	if err := os.WriteFile(src, []byte("aaaacccc"), 0o600); err != nil {
		t.Fatalf("write src: %v", err)
	}
	manifestPath := dir + "/manifest"
	idx, err := manifest.Create(manifestPath, "id", 8, 4, 0, 0, 0, 0)
	if err != nil {
		t.Fatalf("create manifest: %v", err)
	}
	xx0 := xxh3.Hash([]byte("aaaa"))
	d0 := blake3.Sum256([]byte("aaaa"))
	if err := idx.Set(0, 4, 0, xx0, d0); err != nil {
		t.Fatalf("Set: %v", err)
	}
	xx1 := xxh3.Hash([]byte("bbbb"))
	d1 := blake3.Sum256([]byte("bbbb"))
	if err := idx.Set(4, 4, 0, xx1, d1); err != nil {
		t.Fatalf("Set: %v", err)
	}
	idx.Close()

	cfg := &config.Config{ManifestPath: manifestPath}
	core, obs := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	defer logger.Sync()
	if err := estimateTransfer(src, cfg, logger); err != nil {
		t.Fatalf("estimate transfer: %v", err)
	}
	logs := obs.All()
	if len(logs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(logs))
	}
	v := logs[0].ContextMap()["estimated_tx_bytes"].(int64)
	if v != 4 {
		t.Fatalf("expected 4 estimated bytes, got %d", v)
	}
}

func TestEstimateTransferMissingManifest(t *testing.T) {
	src := t.TempDir() + "/src"
	if err := os.WriteFile(src, []byte("data"), 0o600); err != nil {
		t.Fatalf("write src: %v", err)
	}
	cfg := &config.Config{ManifestPath: src + "-missing"}
	if err := estimateTransfer(src, cfg, zap.NewNop()); err == nil {
		t.Fatalf("expected error for missing manifest")
	}
}
