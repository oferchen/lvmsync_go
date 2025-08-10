package config

import (
	"os/exec"
	"testing"
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
