package manifest

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"lvmsync_go/config"
	"lvmsync_go/device"
)

func TestRunDefaultOutputPath(t *testing.T) {
	dir := t.TempDir()
	devicePath := filepath.Join(dir, "dev.img")
	if err := os.WriteFile(devicePath, []byte("data"), 0o600); err != nil {
		t.Fatalf("write device: %v", err)
	}

	restore := device.SetUUIDFunc(func(context.Context, string) (string, error) { return "uuid", nil })
	defer device.SetUUIDFunc(restore)

	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}

	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	if err := Run(cfg, []string{"rebuild", devicePath}, logger); err != nil {
		t.Fatalf("Run: %v", err)
	}

	outPath := devicePath + ".manifest"
	if _, err := os.Stat(outPath); err != nil {
		t.Fatalf("manifest not created at %s: %v", outPath, err)
	}

	if logs.FilterMessage("rebuilding manifest").Len() != 1 {
		t.Fatalf("expected rebuilding manifest log, got %d entries", logs.FilterMessage("rebuilding manifest").Len())
	}
	entry := logs.FilterMessage("rebuilding manifest").All()[0]
	ctx := entry.ContextMap()
	if ctx["device"] != devicePath || ctx["output"] != outPath {
		t.Fatalf("unexpected log fields: %v", ctx)
	}
	if logs.FilterMessage("rebuild complete").Len() != 1 {
		t.Fatalf("expected rebuild complete log")
	}
}

func TestRunManifestPathFlag(t *testing.T) {
	dir := t.TempDir()
	devicePath := filepath.Join(dir, "dev.img")
	if err := os.WriteFile(devicePath, []byte("data"), 0o600); err != nil {
		t.Fatalf("write device: %v", err)
	}

	restore := device.SetUUIDFunc(func(context.Context, string) (string, error) { return "uuid", nil })
	defer device.SetUUIDFunc(restore)

	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}

	outputPath := filepath.Join(dir, "custom.manifest")
	args := []string{"rebuild", "--manifest_path", outputPath, devicePath}
	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	if err := Run(cfg, args, logger); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if _, err := os.Stat(outputPath); err != nil {
		t.Fatalf("manifest not created at %s: %v", outputPath, err)
	}
	if _, err := os.Stat(devicePath + ".manifest"); !os.IsNotExist(err) {
		t.Fatalf("unexpected manifest at default path")
	}

	if logs.FilterMessage("rebuilding manifest").Len() != 1 {
		t.Fatalf("expected rebuilding manifest log, got %d entries", logs.FilterMessage("rebuilding manifest").Len())
	}
	entry := logs.FilterMessage("rebuilding manifest").All()[0]
	ctx := entry.ContextMap()
	if ctx["device"] != devicePath || ctx["output"] != outputPath {
		t.Fatalf("unexpected log fields: %v", ctx)
	}
	if logs.FilterMessage("rebuild complete").Len() != 1 {
		t.Fatalf("expected rebuild complete log")
	}
}

func TestRunMissingArgs(t *testing.T) {
	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}

	cases := []struct {
		name string
		args []string
	}{
		{"missing subcommand", []string{}},
		{"missing device", []string{"rebuild"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var runErr error
			defer func() {
				if r := recover(); r == nil && runErr == nil {
					t.Fatalf("expected failure for args %v", tc.args)
				}
			}()
			runErr = Run(cfg, tc.args, zap.NewNop())
			if runErr == nil {
				t.Fatalf("expected failure for args %v", tc.args)
			}
		})
	}
}
