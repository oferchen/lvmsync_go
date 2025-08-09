package main

import (
	"errors"
	"io"
	"os"
	"testing"

	"lvmsync_go/config"
)

func TestRunLocalDumpSuccess(t *testing.T) {
	var err error
	cfg, err = config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig returned error: %v", err)
	}
	cfg.DedupStrategy = "none"
	cfg.Parallel = 1
	cfg.SpeedLimit = 0

	originalOpen := openFile
	var openCalled bool
	openFile = func(name string, flag int, perm os.FileMode) (*os.File, error) {
		openCalled = true
		if name != "/fake/dest" {
			t.Fatalf("expected dest /fake/dest, got %s", name)
		}
		return os.OpenFile(os.DevNull, os.O_RDWR, 0)
	}
	defer func() { openFile = originalOpen }()

	originalDump := dumpChangesSequential
	var dumpCalled bool
	dumpChangesSequential = func(c *config.Config, snap, origin string, out io.Writer) error {
		dumpCalled = true
		if snap != "snap" || origin != "orig" {
			t.Fatalf("unexpected devices: %s %s", snap, origin)
		}
		return nil
	}
	defer func() { dumpChangesSequential = originalDump }()

	if err := runLocalDump("snap", "orig", "/fake/dest"); err != nil {
		t.Fatalf("runLocalDump returned error: %v", err)
	}
	if !openCalled {
		t.Fatalf("openFile was not called")
	}
	if !dumpCalled {
		t.Fatalf("dumpChangesSequential was not called")
	}
}

func TestRunLocalDumpOpenError(t *testing.T) {
	var err error
	cfg, err = config.DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig returned error: %v", err)
	}

	originalOpen := openFile
	openFile = func(name string, flag int, perm os.FileMode) (*os.File, error) {
		return nil, errors.New("open error")
	}
	defer func() { openFile = originalOpen }()

	originalDump := dumpChangesSequential
	dumpChangesSequential = func(c *config.Config, snap, origin string, out io.Writer) error {
		t.Fatalf("dumpChangesSequential should not be called on open error")
		return nil
	}
	defer func() { dumpChangesSequential = originalDump }()

	if err := runLocalDump("snap", "orig", "/fake/dest"); err == nil {
		t.Fatalf("expected error, got nil")
	}
}
