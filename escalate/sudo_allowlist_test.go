//go:build root

package escalate

import (
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"
)

func TestEnsureRootOrReexec_RejectsDisallowedCommand(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root")
	}

	dir := t.TempDir()
	sudoPath := filepath.Join(dir, "sudo")
	script := "#!/bin/sh\nfor arg in \"$@\"; do\n  if [ \"$arg\" = disallowed ]; then\n    echo disallowed >&2\n    exit 1\n  fi\ndone\nexit 0\n"
	if err := os.WriteFile(sudoPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write sudo stub: %v", err)
	}

	t.Setenv("PATH", dir)

	_, err := EnsureRootOrReexec(Options{
		Geteuid:   func() int { return 1000 },
		ExtraArgs: []string{"disallowed"},
	}, zap.NewNop())
	if err == nil {
		t.Fatal("expected error for disallowed command")
	}
}
