//go:build linux

package device

import (
	"bytes"
	"os"
	"testing"

	"go.uber.org/zap"
	"github.com/oferchen/lvmsync_go/internal/privilege"
)

// TestDiscardLoopDevice verifies that issuing BLKDISCARD on a loop device
// zeroes the specified range and clears privileges when sanitize is true.
func TestDiscardLoopDevice(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root")
	}
	loop, cleanup := setupLoop(t, 1<<20)
	defer cleanup()

	f, err := os.OpenFile(loop, os.O_RDWR, 0)
	if err != nil {
		t.Fatalf("open loop: %v", err)
	}
	defer f.Close()

	data := bytes.Repeat([]byte{0x5a}, 4096)
	if _, err := f.WriteAt(data, 0); err != nil {
		t.Fatalf("write loop: %v", err)
	}
	if err := f.Sync(); err != nil {
		t.Fatalf("sync loop: %v", err)
	}

	hadCaps := privilege.RealHasCaps()
	d := NewDiscarder()
	if err := d.DiscardRange(f, 0, uint64(len(data)), true, false, zap.NewNop()); err != nil {
		t.Fatalf("discard: %v", err)
	}

	buf := make([]byte, len(data))
	if _, err := f.ReadAt(buf, 0); err != nil {
		t.Fatalf("read loop: %v", err)
	}
	if !bytes.Equal(buf, make([]byte, len(data))) {
		t.Fatalf("expected zeroed region after discard")
	}

	if hadCaps && privilege.RealHasCaps() {
		t.Fatalf("privileges not cleared after discard")
	}
}
