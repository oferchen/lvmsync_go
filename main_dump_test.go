package main

import (
	"io"
	"testing"

	"lvmsync_go/config"
	"lvmsync_go/transfer"
)

func TestExecuteDumpSequential(t *testing.T) {
	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig returned error: %v", err)
	}
	cfg.DedupStrategy = "none"
	cfg.Parallel = 1

	called := false
	original := dumpChangesSequential
	dumpChangesSequential = func(c *config.Config, snap, origin string, out io.Writer) error {
		called = true
		return nil
	}
	defer func() { dumpChangesSequential = original }()

	if err := executeDump(cfg, "snap", "orig", io.Discard); err != nil {
		t.Fatalf("executeDump returned error: %v", err)
	}
	if !called {
		t.Fatalf("dumpChangesSequential was not called")
	}
}

func TestExecuteDumpParallel(t *testing.T) {
	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig returned error: %v", err)
	}
	cfg.Parallel = 2

	called := false
	original := dumpChangesParallel
	dumpChangesParallel = func(c *config.Config, snap, origin string, out io.Writer) error {
		called = true
		return nil
	}
	defer func() { dumpChangesParallel = original }()

	if err := executeDump(cfg, "snap", "orig", io.Discard); err != nil {
		t.Fatalf("executeDump returned error: %v", err)
	}
	if !called {
		t.Fatalf("dumpChangesParallel was not called")
	}
}

func TestExecuteDumpWithDedup(t *testing.T) {
	cfg, err := config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig returned error: %v", err)
	}
	cfg.DedupStrategy = "checksum"

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

	if err := executeDump(cfg, "snap", "orig", io.Discard); err != nil {
		t.Fatalf("executeDump returned error: %v", err)
	}
	if !called {
		t.Fatalf("dumpChangesWithDeduplication was not called")
	}
}
