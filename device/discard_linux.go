//go:build linux

package device

import (
	"fmt"
	"os"
	"unsafe"

	"go.uber.org/zap"
	"golang.org/x/sys/unix"

	"lvmsync_go/escalate"
	"lvmsync_go/internal/privilege"
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

var discardImpl = blkdiscard

// SetDiscardFunc overrides the discard implementation. It returns a restore function.
func SetDiscardFunc(fn func(*os.File, uint64, uint64, bool, bool, *zap.Logger) error) func() {
	orig := discardImpl
	if fn == nil {
		discardImpl = blkdiscard
	} else {
		discardImpl = fn
	}
	return func() { discardImpl = orig }
}

// DiscardRange issues BLKDISCARD for the specified range on f.
func DiscardRange(f *os.File, offset, length uint64, sanitize, noNewPrivs bool, logger *zap.Logger) error {
	if logger == nil {
		panic("nil logger")
	}
	return discardImpl(f, offset, length, sanitize, noNewPrivs, logger)
}
