package root

import (
	"encoding/json"
	"io"
	"os"
	"reflect"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/oferchen/lvmsync_go/internal/config"
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
	stdout := os.Stdout
	os.Stdout = wOut
	obs, logs := observer.New(zap.WarnLevel)
	logger := zap.New(obs)
	if e := emitPlan(cfg, []string{f.Name()}, logger); e != nil {
		t.Fatalf("emitPlan: %v", e)
	}
	wOut.Close()
	os.Stdout = stdout
	outBytes, _ := io.ReadAll(rOut)
	if logs.Len() == 0 {
		t.Fatalf("missing warning")
	}
	if logs.All()[0].Message != "allow_insecure enabled; security checks disabled" {
		t.Fatalf("unexpected warning: %v", logs.All()[0].Message)
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
	stdout := os.Stdout
	os.Stdout = wOut
	obs, logs := observer.New(zap.WarnLevel)
	logger := zap.New(obs)
	if e := emitPlan(cfg, []string{f.Name()}, logger); e == nil {
		t.Fatalf("expected error")
	}
	wOut.Close()
	os.Stdout = stdout
	outBytes, _ := io.ReadAll(rOut)
	if logs.Len() == 0 {
		t.Fatalf("missing warning")
	}
	if logs.All()[0].Message != "rsync transport requires --allow-insecure" {
		t.Fatalf("unexpected warning: %v", logs.All()[0].Message)
	}
	if len(outBytes) != 0 {
		t.Fatalf("unexpected plan output: %q", outBytes)
	}
}

func TestEmitPlanRsyncExactMatch(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "src")
	if err != nil {
		t.Fatalf("tempfile: %v", err)
	}
	cases := []struct {
		name      string
		transport string
		warn      bool
		wantErr   bool
	}{
		{name: "exact", transport: "rsync", warn: true, wantErr: true},
		{name: "prefix", transport: "rsyncssh", warn: false, wantErr: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.Config{Transport: tc.transport}
			rOut, wOut, _ := os.Pipe()
			stdout := os.Stdout
			os.Stdout = wOut
			obs, logs := observer.New(zap.WarnLevel)
			logger := zap.New(obs)
			err := emitPlan(cfg, []string{f.Name()}, logger)
			wOut.Close()
			os.Stdout = stdout
			outBytes, _ := io.ReadAll(rOut)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.warn && logs.Len() == 0 {
				t.Fatalf("missing warning")
			}
			if !tc.warn && logs.Len() != 0 {
				t.Fatalf("unexpected warning: %v", logs.All())
			}
			if tc.wantErr {
				if len(outBytes) != 0 {
					t.Fatalf("unexpected plan output: %q", outBytes)
				}
			} else {
				if len(outBytes) == 0 {
					t.Fatalf("expected plan output")
				}
			}
		})
	}
}

func TestEmitPlanRedactsAndOrdersTransports(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "src")
	if err != nil {
		t.Fatalf("tempfile: %v", err)
	}
	cfg := &config.Config{
		SSHPassword:    "pass",
		SSHKeyPath:     "keypath",
		SSHHostKey:     "hostkey",
		SSHHostKeyPath: "hostkeypath",
		KnownHosts:     "known",
		ClientCert:     "cert",
		ClientKey:      "clientkey",
		CACert:         "ca",
		Transport:      "ssh,tcp,tls",
	}
	rOut, wOut, _ := os.Pipe()
	stdout := os.Stdout
	os.Stdout = wOut
	if e := emitPlan(cfg, []string{f.Name()}, zap.NewNop()); e != nil {
		t.Fatalf("emitPlan: %v", e)
	}
	wOut.Close()
	os.Stdout = stdout
	outBytes, _ := io.ReadAll(rOut)
	var po planOutput
	if err := json.Unmarshal(outBytes, &po); err != nil {
		t.Fatalf("decode plan: %v", err)
	}
	if po.Config.SSHPassword != "" || po.Config.SSHKeyPath != "" || po.Config.SSHHostKey != "" || po.Config.SSHHostKeyPath != "" || po.Config.KnownHosts != "" || po.Config.ClientCert != "" || po.Config.ClientKey != "" || po.Config.CACert != "" {
		t.Fatalf("secrets not redacted: %#v", po.Config)
	}
	expected := []string{"ssh", "tcp", "tls"}
	if !reflect.DeepEqual(po.TransportOrder, expected) {
		t.Fatalf("transport order %v != %v", po.TransportOrder, expected)
	}
}

func TestEmitPlanMissingSource(t *testing.T) {
	if err := emitPlan(&config.Config{}, nil, zap.NewNop()); err == nil || err.Error() != "missing source argument" {
		t.Fatalf("expected missing source error, got: %v", err)
	}
}
