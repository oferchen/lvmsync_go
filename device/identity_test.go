package device

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"lvmsync_go/internal/privilege"
)

// verifyIdentity checks that two device paths have matching size and UUID.
func verifyIdentity(ctx context.Context, info *Info, src, dest string) error {
	sizeA, err := info.SizeBytes(ctx, src)
	if err != nil {
		return err
	}
	sizeB, err := info.SizeBytes(ctx, dest)
	if err != nil {
		return err
	}
	if sizeA != sizeB {
		return fmt.Errorf("size mismatch")
	}
	match, err := info.IDsMatch(ctx, src, dest)
	if err != nil {
		return err
	}
	if !match {
		return fmt.Errorf("uuid mismatch")
	}
	return nil
}

type identityStub struct{ size uint64 }

func (s *identityStub) Path() string                                     { return "" }
func (s *identityStub) SizeBytes() uint64                                { return s.size }
func (s *identityStub) BlockSize() uint64                                { return 0 }
func (s *identityStub) Snapshot(context.Context, string) (Device, error) { return s, nil }
func (s *identityStub) Cleanup(context.Context) error                    { return nil }
func (s *identityStub) Close() error                                     { return nil }
func (s *identityStub) Identity(context.Context) (DeviceIdentity, error) {
	return DeviceIdentity{SizeBytes: s.size}, nil
}
func (s *identityStub) AppendWAL(r Range) error               { return nil }
func (s *identityStub) RecoverWAL(fn func(Range) error) error { return nil }

func TestIdentitySizeMismatchFailsEarly(t *testing.T) {
	info := NewInfo()
	prev := info.SetDetectFunc(func(_ context.Context, path string, _ bool, _, _, _, _ string, _ time.Duration, _ time.Duration, _ privilege.Escalator, _ *zap.Logger, _ *Runner) (Device, error) {
		if strings.Contains(path, "src") {
			return &identityStub{size: 1}, nil
		}
		return &identityStub{size: 2}, nil
	})
	defer info.SetDetectFunc(prev)
	info.uuidFunc = func(context.Context, string) (string, error) {
		t.Fatalf("uuid check invoked despite size mismatch")
		return "", nil
	}
	if err := verifyIdentity(context.Background(), info, "/dev/src", "/dev/dest"); err == nil || !strings.Contains(err.Error(), "size mismatch") {
		t.Fatalf("expected size mismatch error, got %v", err)
	}
}

func TestIdentityFSUUIDMismatch(t *testing.T) {
	info := NewInfoWithDeps(func(_ context.Context, path string) (string, error) {
		if strings.Contains(path, "src") {
			return "id1", nil
		}
		return "id2", nil
	}, nil, nil, nil, nil)
	prev := info.SetDetectFunc(func(context.Context, string, bool, string, string, string, string, time.Duration, time.Duration, privilege.Escalator, *zap.Logger, *Runner) (Device, error) {
		return &identityStub{size: 1}, nil
	})
	defer info.SetDetectFunc(prev)
	if err := verifyIdentity(context.Background(), info, "/dev/src", "/dev/dest"); err == nil || !strings.Contains(err.Error(), "uuid mismatch") {
		t.Fatalf("expected fs uuid mismatch error, got %v", err)
	}
}

func TestDeviceIdentityFormatParseOrder(t *testing.T) {
	id := DeviceIdentity{
		SizeBytes:     1,
		KernelUUID:    "k",
		GPTUUID:       "g",
		MBRSignature:  "m",
		FSUUID:        "f",
		Major:         2,
		Minor:         3,
		ManifestEpoch: 4,
	}
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "%d %s %s %s %s %d %d %d", id.SizeBytes, id.KernelUUID, id.GPTUUID, id.MBRSignature, id.FSUUID, id.Major, id.Minor, id.ManifestEpoch)
	var parsed DeviceIdentity
	if _, err := fmt.Fscan(&buf, &parsed.SizeBytes, &parsed.KernelUUID, &parsed.GPTUUID, &parsed.MBRSignature, &parsed.FSUUID, &parsed.Major, &parsed.Minor, &parsed.ManifestEpoch); err != nil {
		t.Fatalf("Fscan: %v", err)
	}
	if parsed != id {
		t.Fatalf("round-trip mismatch: got %+v want %+v", parsed, id)
	}
}

func createEchoScript(t *testing.T, name, output string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	script := fmt.Sprintf("#!/bin/sh\necho %s\n", output)
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	return path
}

func TestRawIdentityKernelAndFSUUID(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "dev")
	if err != nil {
		t.Fatalf("tempfile: %v", err)
	}
	defer f.Close()
	d := &RawDevice{f: f, logger: zap.NewNop()}

	origLSBLK := lsblkPath
	origBLKID := blkidPath
	lsblkPath = createEchoScript(t, "lsblk", "kernel")
	blkidPath = createEchoScript(t, "blkid", "fs")
	defer func() {
		lsblkPath = origLSBLK
		blkidPath = origBLKID
	}()

	id, err := d.Identity(context.Background())
	if err != nil {
		t.Fatalf("Identity: %v", err)
	}
	if id.KernelUUID != "kernel" {
		t.Fatalf("kernel uuid mismatch: %v", id.KernelUUID)
	}
	if id.FSUUID != "fs" {
		t.Fatalf("fs uuid mismatch: %v", id.FSUUID)
	}
}
