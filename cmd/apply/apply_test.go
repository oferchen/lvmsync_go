package apply

import (
	"testing"

	"lvmsync_go/config"
)

func TestRun(t *testing.T) {
	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig returned error: %v", err)
	}
	applyFile := "dumpfile"
	dest := "/dev/null"

	called := false
	original := applyFunc
	applyFunc = func(c *config.Config, applyFileArg, destDevice string) error {
		called = true
		if applyFileArg != applyFile {
			t.Fatalf("expected applyFile %s, got %s", applyFile, applyFileArg)
		}
		if destDevice != dest {
			t.Fatalf("expected dest %s, got %s", dest, destDevice)
		}
		return nil
	}
	defer func() { applyFunc = original }()

	if err := Run(cfg, applyFile, []string{dest}); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !called {
		t.Fatalf("applyFunc was not called")
	}
}
