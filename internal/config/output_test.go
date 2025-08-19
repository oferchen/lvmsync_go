package config

import "testing"

func TestValidateOutput(t *testing.T) {
	cfg, err := DefaultConfig()
	if err != nil {
		t.Fatalf("default config: %v", err)
	}
	cfg.Output = "xml"
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected error for invalid output")
	}
	cfg.Output = "json"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("validate json output: %v", err)
	}
	cfg.Output = "yaml"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("validate yaml output: %v", err)
	}
}
