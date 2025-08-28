package device

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/oferchen/lvmsync_go/hash"
	"github.com/oferchen/lvmsync_go/internal/privilege"
)

type identityStub struct {
	size  uint64
	epoch uint64
	gpt   string
	mbr   string
	phash [32]byte
}

func (s *identityStub) Path() string                                     { return "" }
func (s *identityStub) SizeBytes() uint64                                { return s.size }
func (s *identityStub) BlockSize() uint64                                { return 0 }
func (s *identityStub) Snapshot(context.Context, string) (Device, error) { return s, nil }
func (s *identityStub) Cleanup(context.Context) error                    { return nil }
func (s *identityStub) Close() error                                     { return nil }
func (s *identityStub) Identity(context.Context) (DeviceIdentity, error) {
	return DeviceIdentity{SizeBytes: s.size, GPTUUID: s.gpt, MBRSignature: s.mbr, ManifestEpoch: s.epoch, PartitionHash: s.phash}, nil
}
func (s *identityStub) AppendWAL(_ Range) error              { return nil }
func (s *identityStub) RecoverWAL(_ func(Range) error) error { return nil }

func TestVerifyIdentityMatch(t *testing.T) {
	info := NewInfoWithDeps(func(context.Context, string) (string, error) { return "id", nil }, nil, nil, nil, nil)
	prev := info.SetDetectFunc(func(context.Context, string, bool, bool, string, string, string, string, time.Duration, time.Duration, privilege.Escalator, *zap.Logger, *Runner) (Device, error) {
		return &identityStub{size: 1, epoch: 1}, nil
	})
	defer info.SetDetectFunc(prev)
	if err := VerifyIdentity(context.Background(), info, "/dev/src", "/dev/dest"); err != nil {
		t.Fatalf("VerifyIdentity: %v", err)
	}
}

func TestIdentitySizeMismatchFailsEarly(t *testing.T) {
	info := NewInfo()
	prev := info.SetDetectFunc(func(_ context.Context, path string, _ bool, _ bool, _, _, _, _ string, _ time.Duration, _ time.Duration, _ privilege.Escalator, _ *zap.Logger, _ *Runner) (Device, error) {
		if strings.Contains(path, "src") {
			return &identityStub{size: 1, epoch: 1}, nil
		}
		return &identityStub{size: 2, epoch: 1}, nil
	})
	defer info.SetDetectFunc(prev)
	info.uuidFunc = func(context.Context, string) (string, error) {
		t.Fatalf("uuid check invoked despite size mismatch")
		return "", nil
	}
	if err := VerifyIdentity(context.Background(), info, "/dev/src", "/dev/dest"); err == nil || !strings.Contains(err.Error(), "size mismatch") {
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
	prev := info.SetDetectFunc(func(context.Context, string, bool, bool, string, string, string, string, time.Duration, time.Duration, privilege.Escalator, *zap.Logger, *Runner) (Device, error) {
		return &identityStub{size: 1, epoch: 1}, nil
	})
	defer info.SetDetectFunc(prev)
	if err := VerifyIdentity(context.Background(), info, "/dev/src", "/dev/dest"); err == nil || !strings.Contains(err.Error(), "uuid mismatch") {
		t.Fatalf("expected fs uuid mismatch error, got %v", err)
	}
}

func TestIdentityGPTUUIDMismatch(t *testing.T) {
	info := NewInfoWithDeps(func(context.Context, string) (string, error) { return "id", nil }, nil, nil, nil, nil)
	prev := info.SetDetectFunc(func(_ context.Context, path string, _ bool, _ bool, _, _, _, _ string, _ time.Duration, _ time.Duration, _ privilege.Escalator, _ *zap.Logger, _ *Runner) (Device, error) {
		if strings.Contains(path, "src") {
			return &identityStub{size: 1, epoch: 1, gpt: "gpt1", mbr: "m"}, nil
		}
		return &identityStub{size: 1, epoch: 1, gpt: "gpt2", mbr: "m"}, nil
	})
	defer info.SetDetectFunc(prev)
	if err := VerifyIdentity(context.Background(), info, "/dev/src", "/dev/dest"); err == nil || !strings.Contains(err.Error(), "gpt uuid mismatch") {
		t.Fatalf("expected gpt uuid mismatch error, got %v", err)
	}
}

func TestIdentityMBRSignatureMismatch(t *testing.T) {
	info := NewInfoWithDeps(func(context.Context, string) (string, error) { return "id", nil }, nil, nil, nil, nil)
	prev := info.SetDetectFunc(func(_ context.Context, path string, _ bool, _ bool, _, _, _, _ string, _ time.Duration, _ time.Duration, _ privilege.Escalator, _ *zap.Logger, _ *Runner) (Device, error) {
		if strings.Contains(path, "src") {
			return &identityStub{size: 1, epoch: 1, gpt: "g", mbr: "m1"}, nil
		}
		return &identityStub{size: 1, epoch: 1, gpt: "g", mbr: "m2"}, nil
	})
	defer info.SetDetectFunc(prev)
	if err := VerifyIdentity(context.Background(), info, "/dev/src", "/dev/dest"); err == nil || !strings.Contains(err.Error(), "mbr signature mismatch") {
		t.Fatalf("expected mbr signature mismatch error, got %v", err)
	}
}

func TestIdentityPartitionHashMismatch(t *testing.T) {
	info := NewInfoWithDeps(func(context.Context, string) (string, error) { return "id", nil }, nil, nil, nil, nil)
	prev := info.SetDetectFunc(func(_ context.Context, path string, _ bool, _ bool, _, _, _, _ string, _ time.Duration, _ time.Duration, _ privilege.Escalator, _ *zap.Logger, _ *Runner) (Device, error) {
		if strings.Contains(path, "src") {
			return &identityStub{size: 1, epoch: 1, phash: hash.SumBLAKE3([]byte("a"))}, nil
		}
		return &identityStub{size: 1, epoch: 1, phash: hash.SumBLAKE3([]byte("b"))}, nil
	})
	defer info.SetDetectFunc(prev)
	if err := VerifyIdentity(context.Background(), info, "/dev/src", "/dev/dest"); err == nil || !strings.Contains(err.Error(), "partition hash mismatch") {
		t.Fatalf("expected partition hash mismatch error, got %v", err)
	}
}

func TestVerifyIdentityManifestEpochMismatch(t *testing.T) {
	info := NewInfoWithDeps(func(context.Context, string) (string, error) { return "id", nil }, nil, nil, nil, nil)
	prev := info.SetDetectFunc(func(_ context.Context, path string, _ bool, _ bool, _, _, _, _ string, _ time.Duration, _ time.Duration, _ privilege.Escalator, _ *zap.Logger, _ *Runner) (Device, error) {
		if strings.Contains(path, "src") {
			return &identityStub{size: 1, epoch: 1}, nil
		}
		return &identityStub{size: 1, epoch: 2}, nil
	})
	defer info.SetDetectFunc(prev)
	if err := VerifyIdentity(context.Background(), info, "/dev/src", "/dev/dest"); err == nil || !strings.Contains(err.Error(), "manifest epoch mismatch") {
		t.Fatalf("expected manifest epoch mismatch error, got %v", err)
	}
}

func TestDeviceIdentityFormatParseOrder(t *testing.T) {
	id := DeviceIdentity{
		SizeBytes:     1,
		KernelUUID:    "k",
		GPTUUID:       "g",
		MBRSignature:  "m",
		FSUUID:        "f",
		PartitionHash: hash.SumBLAKE3([]byte("p")),
		Major:         2,
		Minor:         3,
		ManifestEpoch: 4,
	}
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "%d %s %s %s %s %x %d %d %d", id.SizeBytes, id.KernelUUID, id.GPTUUID, id.MBRSignature, id.FSUUID, id.PartitionHash, id.Major, id.Minor, id.ManifestEpoch)
	var parsed DeviceIdentity
	var ph string
	if _, err := fmt.Fscan(&buf, &parsed.SizeBytes, &parsed.KernelUUID, &parsed.GPTUUID, &parsed.MBRSignature, &parsed.FSUUID, &ph, &parsed.Major, &parsed.Minor, &parsed.ManifestEpoch); err != nil {
		t.Fatalf("Fscan: %v", err)
	}
	b, err := hex.DecodeString(ph)
	if err != nil || len(b) != 32 {
		t.Fatalf("decode partition hash: %v", err)
	}
	copy(parsed.PartitionHash[:], b)
	if parsed != id {
		t.Fatalf("round-trip mismatch: got %+v want %+v", parsed, id)
	}
}

func TestSameIdentityStrictDetectsMajorMinorMismatch(t *testing.T) {
	base := DeviceIdentity{
		SizeBytes:     1,
		KernelUUID:    "k",
		GPTUUID:       "g",
		MBRSignature:  "m",
		FSUUID:        "f",
		Major:         2,
		Minor:         3,
		ManifestEpoch: 4,
	}

	b := base
	b.Major++
	if SameIdentityStrict(base, b) {
		t.Fatalf("expected major mismatch: %+v vs %+v", base, b)
	}

	b = base
	b.Minor++
	if SameIdentityStrict(base, b) {
		t.Fatalf("expected minor mismatch: %+v vs %+v", base, b)
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
