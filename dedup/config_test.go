package dedup

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTempConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestLoadConfigPrecedence(t *testing.T) {
	cfgPath := writeTempConfig(t, "min_chunk_size: 4096\nmax_chunk_size: 8192\n")

	t.Run("env_overrides_config", func(t *testing.T) {
		t.Setenv("LVMSYNC_MAX_CHUNK_SIZE", "16384")
		cfg, err := LoadConfig(cfgPath, nil)
		if err != nil {
			t.Fatalf("LoadConfig: %v", err)
		}
		if cfg.MaxChunkSize != 16384 {
			t.Fatalf("expected max_chunk_size 16384, got %d", cfg.MaxChunkSize)
		}
	})

	t.Run("flags_override_env", func(t *testing.T) {
		t.Setenv("LVMSYNC_MIN_CHUNK_SIZE", "2048")
		cfg, err := LoadConfig(cfgPath, []string{"--min-chunk-size", "1024"})
		if err != nil {
			t.Fatalf("LoadConfig: %v", err)
		}
		if cfg.MinChunkSize != 1024 {
			t.Fatalf("expected min_chunk_size 1024, got %d", cfg.MinChunkSize)
		}
	})
}

func TestLoadConfigFailures(t *testing.T) {
	t.Run("invalid-yaml", func(t *testing.T) {
		cfgPath := writeTempConfig(t, ":\n")
		if _, err := LoadConfig(cfgPath, nil); err == nil {
			t.Fatalf("expected error for invalid YAML")
		}
	})

	t.Run("parse-error", func(t *testing.T) {
		if _, err := LoadConfig("", []string{"--min-chunk-size", "abc"}); err == nil {
			t.Fatalf("expected parse error")
		}
	})
}
