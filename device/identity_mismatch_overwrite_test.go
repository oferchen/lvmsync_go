//go:build linux

package device

import (
	"context"
	"os"
	"strings"
	"testing"

	"go.uber.org/zap"
)

func TestIdentityMismatchRequiresOverwriteFlags(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root")
	}
	srcLoop, cleanupSrc := setupLoop(t, 1<<20)
	defer cleanupSrc()
	dstLoop, cleanupDst := setupLoop(t, 2<<20)
	defer cleanupDst()

	ctx := WithForce(context.Background(), true)
	if _, err := OpenRaw(ctx, dstLoop, true, "", nil, "", nil, 0, 0, fakeEsc{}, zap.NewNop(), NewRunner()); err == nil || !strings.Contains(err.Error(), "--allow-overwrite") {
		t.Fatalf("expected allow-overwrite error, got %v", err)
	}
	ctx = WithAllowOverwrite(ctx, true)
	if _, err := OpenRaw(ctx, dstLoop, true, "", nil, "", nil, 0, 0, fakeEsc{}, zap.NewNop(), NewRunner()); err == nil || !strings.Contains(err.Error(), "--yes-i-know") {
		t.Fatalf("expected yes-i-know error, got %v", err)
	}
	ctx = WithYesIKnow(ctx, true)
	dev, err := OpenRaw(ctx, dstLoop, true, "", nil, "", nil, 0, 0, fakeEsc{}, zap.NewNop(), NewRunner())
	if err != nil {
		t.Fatalf("OpenRaw: %v", err)
	}
	dev.Close()

	info := NewInfo()
	if err := verifyIdentity(context.Background(), info, srcLoop, dstLoop); err == nil || !strings.Contains(err.Error(), "size mismatch") {
		t.Fatalf("expected size mismatch error, got %v", err)
	}
}
