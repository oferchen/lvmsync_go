//go:build linux

// Package device provides Linux device utilities.
package device

import (
	"fmt"
	"os"
	"unsafe"

	"go.uber.org/zap"
	"golang.org/x/sys/unix"

	"github.com/oferchen/lvmsync_go/escalate"
	"github.com/oferchen/lvmsync_go/internal/privilege"
)

func blkdiscard(f *os.File, offset, length uint64, sanitize, noNewPrivs bool, logger *zap.Logger) error {
	if reexeced, err := escalate.EnsureRootOrReexec(escalate.Options{SanitizeEnv: &sanitize, NoNewPrivs: noNewPrivs}, logger); err != nil {
		return err
	} else if reexeced {
		return fmt.Errorf("re-exec requested for root")
	}
	data := [2]uint64{offset, length}
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, f.Fd(), unix.BLKDISCARD, uintptr(unsafe.Pointer(&data)))
	if errno != 0 {
		return errno
	}
	privilege.ClearAmbient()
	return nil
}

// Discarder issues block discard operations using a configurable implementation.
type Discarder struct {
	fn func(*os.File, uint64, uint64, bool, bool, *zap.Logger) error
}

// NewDiscarder returns a Discarder that uses the real blkdiscard implementation.
func NewDiscarder() *Discarder {
	return &Discarder{fn: blkdiscard}
}

// NewDiscarderWithFunc returns a Discarder that uses fn. If fn is nil, blkdiscard is used.
func NewDiscarderWithFunc(fn func(*os.File, uint64, uint64, bool, bool, *zap.Logger) error) *Discarder {
	if fn == nil {
		fn = blkdiscard
	}
	return &Discarder{fn: fn}
}

// DiscardRange issues BLKDISCARD for the specified range on f. logger must be non-nil.
func (d *Discarder) DiscardRange(f *os.File, offset, length uint64, sanitize, noNewPrivs bool, logger *zap.Logger) error {
	if d == nil {
		return fmt.Errorf("discarder is nil")
	}
	if logger == nil {
		return fmt.Errorf("logger is nil")
	}
	return d.fn(f, offset, length, sanitize, noNewPrivs, logger)
}
