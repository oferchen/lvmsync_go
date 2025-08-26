package root

import (
	"testing"

	"lvmsync_go/internal/config"
)

func TestValidateArgsSuccess(t *testing.T) {
	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig error: %v", err)
	}
	orig, dest, err := validateArgs(cfg, []string{"/src", "/dst"})
	if err != nil {
		t.Fatalf("validateArgs error: %v", err)
	}
	if orig != "/src" || dest != "/dst" {
		t.Fatalf("unexpected results: %s %s", orig, dest)
	}
}

func TestValidateArgsFailure(t *testing.T) {
	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig error: %v", err)
	}
	if _, _, err := validateArgs(cfg, []string{"/src"}); err == nil || err.Error() != "invalid arguments" {
		t.Fatalf("expected invalid arguments error, got: %v", err)
	}
}
