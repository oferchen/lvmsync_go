package manifest

import (
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"

	"lvmsync_go/config"
	"lvmsync_go/device"
)

func TestRunDefaultOutputPath(t *testing.T) {
	dir := t.TempDir()
	devicePath := filepath.Join(dir, "dev.img")
	if err := os.WriteFile(devicePath, []byte("data"), 0o600); err != nil {
		t.Fatalf("write device: %v", err)
	}

	restore := device.SetUUIDFunc(func(string) (string, error) { return "uuid", nil })
	defer device.SetUUIDFunc(restore)

	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}

	if err := Run(cfg, []string{"rebuild", devicePath}, zap.NewNop()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	outPath := devicePath + ".manifest"
	if _, err := os.Stat(outPath); err != nil {
		t.Fatalf("manifest not created at %s: %v", outPath, err)
	}
}

func TestRunOutputFlag(t *testing.T) {
	dir := t.TempDir()
	devicePath := filepath.Join(dir, "dev.img")
	if err := os.WriteFile(devicePath, []byte("data"), 0o600); err != nil {
		t.Fatalf("write device: %v", err)
	}

	restore := device.SetUUIDFunc(func(string) (string, error) { return "uuid", nil })
	defer device.SetUUIDFunc(restore)

	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}

	outputPath := filepath.Join(dir, "custom.manifest")
	args := []string{"rebuild", "--output", outputPath, devicePath}
	if err := Run(cfg, args, zap.NewNop()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if _, err := os.Stat(outputPath); err != nil {
		t.Fatalf("manifest not created at %s: %v", outputPath, err)
	}
	if _, err := os.Stat(devicePath + ".manifest"); !os.IsNotExist(err) {
		t.Fatalf("unexpected manifest at default path")
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
