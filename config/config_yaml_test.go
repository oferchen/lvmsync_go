package config

import (
	"os"
	"os/exec"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestConfigYAMLLint(t *testing.T) {
	if _, err := exec.LookPath("yamllint"); err != nil {
		t.Skip("yamllint not installed")
	}
	cfg := "{extends: default, rules: {brackets: {min-spaces-inside-empty: 1, max-spaces-inside-empty: 1}}}"
	cmd := exec.Command("yamllint", "-d", cfg, "../config.yaml")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("yamllint reported issues: %v\nOutput: %s", err, output)
	} else if len(output) > 0 {
		t.Fatalf("yamllint produced warnings:\n%s", output)
	}
}

func TestConfigYAMLContainsGroups(t *testing.T) {
	data, err := os.ReadFile("../config.yaml")
	if err != nil {
		t.Fatalf("read config.yaml: %v", err)
	}
	var cfg map[string]any
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	keys := []string{"dedup_strategy", "compress", "transport", "lvm_escalation", "grpc_port"}
	for _, k := range keys {
		if _, ok := cfg[k]; !ok {
			t.Fatalf("missing %s key", k)
		}
	}
}
