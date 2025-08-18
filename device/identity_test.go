package device

import (
	"context"
	"fmt"
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

func TestIdentityUUIDMismatch(t *testing.T) {
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
		t.Fatalf("expected uuid mismatch error, got %v", err)
	}
}
