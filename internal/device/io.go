// Package device contains low-level helpers for opening block devices with
// direct I/O and aligned read/write primitives.
package device

import (
	"os"

	"golang.org/x/sys/unix"
)

// Reader wraps the ReadAt method of an io.ReaderAt compatible type.
type Reader interface {
	ReadAt(p []byte, off int64) (int, error)
}

// Writer writes at arbitrary offsets and supports syncing and optional discard.
type Writer interface {
	WriteAt(p []byte, off int64) (int, error)
	Sync() error
	Discard(off, length int64) error
}

// OpenDirect opens path using O_DIRECT for minimal caching. The file must be
// closed by the caller.
func OpenDirect(path string) (*os.File, error) {
	fd, err := unix.Open(path, unix.O_RDWR|unix.O_DIRECT, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), path), nil
}

// FileWriter provides aligned writes and optional discard support.
type FileWriter struct{ *os.File }

// WriteAt writes bytes at the given offset.
func (w *FileWriter) WriteAt(p []byte, off int64) (int, error) { return w.File.WriteAt(p, off) }

// Sync flushes the underlying file.
func (w *FileWriter) Sync() error { return w.File.Sync() }

// Discard punches a hole in the file when supported. A nil error is returned
// if the operation is unsupported.
func (w *FileWriter) Discard(off, length int64) error {
	err := unix.Fallocate(int(w.File.Fd()), unix.FALLOC_FL_PUNCH_HOLE|unix.FALLOC_FL_KEEP_SIZE, off, length)
	if err == unix.EOPNOTSUPP || err == unix.ENOSYS { // syscall may be missing
		return nil
	}
	return err
}
