package dump

import (
	"io"
	"testing"

	"go.uber.org/zap"

	"lvmsync_go/config"
	"lvmsync_go/transfer"
)

func TestExecuteDumpSequential(t *testing.T) {
	cfg2, cfgErr := config.DefaultConfig()
	if cfgErr != nil {
		t.Fatalf("DefaultConfig returned error: %v", cfgErr)
	}
	cfg2.DedupStrategy = "none"
	cfg2.Parallel = 1

	called := false
	original := dumpChangesSequential
	dumpChangesSequential = func(c *config.Config, snap, origin string, out io.Writer) error {
		called = true
		return nil
	}
	defer func() { dumpChangesSequential = original }()

	if execErr := ExecuteDump(cfg2, "snap", "orig", io.Discard, zap.NewNop()); execErr != nil {
		t.Fatalf("executeDump returned error: %v", execErr)
	}
	if !called {
		t.Fatalf("dumpChangesSequential was not called")
	}
}

func TestExecuteDumpParallel(t *testing.T) {
	cfg2, cfgErr := config.DefaultConfig()
	if cfgErr != nil {
		t.Fatalf("DefaultConfig returned error: %v", cfgErr)
	}
	cfg2.Parallel = 2

	called := false
	original := dumpChangesParallel
	dumpChangesParallel = func(c *config.Config, snap, origin string, out io.Writer) error {
		called = true
		return nil
	}
	defer func() { dumpChangesParallel = original }()

	if execErr := ExecuteDump(cfg2, "snap", "orig", io.Discard, zap.NewNop()); execErr != nil {
		t.Fatalf("executeDump returned error: %v", execErr)
	}
	if !called {
		t.Fatalf("dumpChangesParallel was not called")
	}
}

func TestExecuteDumpWithDedup(t *testing.T) {
	cfg2, cfgErr := config.DefaultConfig()
	if cfgErr != nil {
		t.Fatalf("DefaultConfig returned error: %v", cfgErr)
	}
	cfg2.DedupStrategy = "checksum"

	called := false
	original := dumpChangesWithDeduplication
	dumpChangesWithDeduplication = func(c *config.Config, snap, origin string, out io.Writer, d transfer.DeduplicationStrategy) error {
		if d == nil {
			t.Fatalf("deduplication strategy was nil")
		}
		called = true
		return nil
	}
	defer func() { dumpChangesWithDeduplication = original }()

	if execErr := ExecuteDump(cfg2, "snap", "orig", io.Discard, zap.NewNop()); execErr != nil {
		t.Fatalf("executeDump returned error: %v", execErr)
	}
	if !called {
		t.Fatalf("dumpChangesWithDeduplication was not called")
	}
}
