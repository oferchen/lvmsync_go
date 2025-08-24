package root

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigurePrecedence(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfgFile := filepath.Join(t.TempDir(), "cfg.yaml")
	if err := os.WriteFile(cfgFile, []byte("tcp_port: 1111\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Run("flag_overrides_env_yaml", func(t *testing.T) {
		t.Setenv("LVMSYNC_TCP_PORT", "2222")
		orig := os.Args
		os.Args = []string{"cmd", "--config", cfgFile, "--tcp-port", "3333", "/src", "/dst"}
		defer func() { os.Args = orig }()
		cfg, _, _, err := ConfigureWithEscalator(stubEscalator{})
		if err != nil {
			t.Fatalf("Configure error: %v", err)
		}
		if cfg.TCPPort != 3333 {
			t.Fatalf("TCPPort=%d want 3333", cfg.TCPPort)
		}
	})
	t.Run("env_overrides_yaml", func(t *testing.T) {
		t.Setenv("LVMSYNC_TCP_PORT", "2222")
		orig := os.Args
		os.Args = []string{"cmd", "--config", cfgFile, "/src", "/dst"}
		defer func() { os.Args = orig }()
		cfg, _, _, err := ConfigureWithEscalator(stubEscalator{})
		if err != nil {
			t.Fatalf("Configure error: %v", err)
		}
		if cfg.TCPPort != 2222 {
			t.Fatalf("TCPPort=%d want 2222", cfg.TCPPort)
		}
	})
}
