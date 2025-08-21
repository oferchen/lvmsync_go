package root

import (
	"io"
	"os"
	"strings"
	"testing"

	"go.uber.org/zap"

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

func TestEmitPlanAllowInsecureWarns(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "src")
	if err != nil {
		t.Fatalf("tempfile: %v", err)
	}
	cfg := &config.Config{AllowInsecure: true}
	rOut, wOut, _ := os.Pipe()
	rErr, wErr, _ := os.Pipe()
	stdout, stderr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = wOut, wErr
	defer func() { os.Stdout, os.Stderr = stdout, stderr }()
	if e := emitPlan(cfg, []string{f.Name()}, zap.NewNop()); e != nil {
		t.Fatalf("emitPlan: %v", e)
	}
	wOut.Close()
	wErr.Close()
	outBytes, _ := io.ReadAll(rOut)
	errBytes, _ := io.ReadAll(rErr)
	if !strings.Contains(string(errBytes), "allow_insecure enabled") {
		t.Fatalf("missing warning: %q", errBytes)
	}
	if len(outBytes) == 0 {
		t.Fatalf("expected plan output")
	}
}

func TestEmitPlanRsyncRequiresAllowInsecure(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "src")
	if err != nil {
		t.Fatalf("tempfile: %v", err)
	}
	cfg := &config.Config{Transport: "rsync"}
	rOut, wOut, _ := os.Pipe()
	rErr, wErr, _ := os.Pipe()
	stdout, stderr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = wOut, wErr
	defer func() { os.Stdout, os.Stderr = stdout, stderr }()
	if e := emitPlan(cfg, []string{f.Name()}, zap.NewNop()); e == nil {
		t.Fatalf("expected error")
	}
	wOut.Close()
	wErr.Close()
	outBytes, _ := io.ReadAll(rOut)
	errBytes, _ := io.ReadAll(rErr)
	if !strings.Contains(string(errBytes), "allow_insecure enabled") {
		t.Fatalf("missing warning: %q", errBytes)
	}
	if len(outBytes) != 0 {
		t.Fatalf("unexpected plan output: %q", outBytes)
	}
}
