package root

import (
	"testing"

	"lvmsync_go/internal/config"
)

func TestRedactConfig(t *testing.T) {
	cfg := &config.Config{
		SSHPassword:    "pass",
		SSHKeyPath:     "sshkey",
		SSHHostKey:     "hostkey",
		SSHHostKeyPath: "hostkeypath",
		KnownHosts:     "known",
		ClientCert:     "cert",
		ClientKey:      "key",
		CACert:         "ca",
	}
	red := redactConfig(cfg)
	if red.SSHPassword != "" || red.SSHKeyPath != "" || red.SSHHostKey != "" || red.SSHHostKeyPath != "" || red.KnownHosts != "" || red.ClientCert != "" || red.ClientKey != "" || red.CACert != "" {
		t.Fatalf("expected secrets redacted: %#v", red)
	}
	if cfg.SSHPassword == "" || cfg.ClientKey == "" {
		t.Fatalf("original config modified: %#v", cfg)
	}
}
