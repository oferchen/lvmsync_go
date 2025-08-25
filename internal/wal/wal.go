package wal

import (
	"encoding/binary"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// Range represents a byte range.
type Range struct {
	Start uint64
	End   uint64
}

// File captures the subset of *os.File methods used by WAL operations.
type File interface {
	io.ReaderAt
	io.Writer
	io.WriterAt
	io.Seeker
	Sync() error
	Truncate(int64) error
	Close() error
	Stat() (fs.FileInfo, error)
	Name() string
}

// Deps bundles filesystem helpers for WAL operations.
type Deps struct {
	syncDir  func(string) error
	openFile func(string, int, os.FileMode) (File, error)
	rename   func(string, string) error
}

// NewDeps returns production dependencies.
func NewDeps() *Deps {
	return &Deps{
		syncDir:  syncDir,
		openFile: func(name string, flag int, perm os.FileMode) (File, error) { return os.OpenFile(name, flag, perm) },
		rename:   os.Rename,
	}
}

// SyncDir fsyncs the provided directory path.
func (d *Deps) SyncDir(path string) error { return d.syncDir(path) }

// OpenFile opens or creates a file.
func (d *Deps) OpenFile(name string, flag int, perm os.FileMode) (File, error) {
	return d.openFile(name, flag, perm)
}

// Rename moves oldpath to newpath.
func (d *Deps) Rename(oldpath, newpath string) error { return d.rename(oldpath, newpath) }

// NewDepsWithSync returns dependencies using the provided syncDir implementation.
func NewDepsWithSync(fn func(string) error) *Deps {
	d := NewDeps()
	d.syncDir = fn
	return d
}

// WAL manages a write-ahead log backed by a file.
type WAL struct {
	f    File
	deps *Deps
}

// New returns a WAL using the provided file and dependencies. If deps is nil
// production defaults are used.
func New(f File, deps *Deps) *WAL {
	if deps == nil {
		deps = NewDeps()
	}
	return &WAL{f: f, deps: deps}
}

// Append records r using a temporary file and atomic rename for crash safety.
func (w *WAL) Append(r Range) error {
	if r.Start > r.End {
		return fmt.Errorf("wal: invalid range: start %d > end %d", r.Start, r.End)
	}
	name := w.f.Name()
	tmpPath := name + ".tmp"
	tf, err := w.deps.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := w.f.Seek(0, 0); err != nil {
		tf.Close()
		os.Remove(tmpPath)
		return err
	}
	st, err := w.f.Stat()
	if err != nil {
		tf.Close()
		os.Remove(tmpPath)
		return err
	}
	if _, err := io.Copy(tf, io.NewSectionReader(w.f, 0, st.Size())); err != nil {
		tf.Close()
		os.Remove(tmpPath)
		return err
	}
	var buf [16]byte
	binary.LittleEndian.PutUint64(buf[0:8], r.Start)
	binary.LittleEndian.PutUint64(buf[8:16], r.End)
	if n, err := tf.Write(buf[:]); err != nil {
		tf.Close()
		os.Remove(tmpPath)
		return err
	} else if n != len(buf) {
		tf.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("wal: short write: wrote %d of %d bytes", n, len(buf))
	}
	if err := tf.Sync(); err != nil {
		tf.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tf.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	if err := w.f.Close(); err != nil {
		return err
	}
	if err := w.deps.Rename(tmpPath, name); err != nil {
		return err
	}
	nf, err := w.deps.OpenFile(name, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	if _, err := nf.Seek(0, io.SeekEnd); err != nil {
		nf.Close()
		return err
	}
	w.f = nf
	return w.deps.syncDir(filepath.Dir(name))
}

// Sync flushes the WAL to stable storage.
func (w *WAL) Sync() error {
	if w.f != nil {
		return w.f.Sync()
	}
	return nil
}

// Close flushes the WAL and fsyncs its parent directory.
func (w *WAL) Close() error {
	if w.f == nil {
		return nil
	}
	if err := w.f.Sync(); err != nil {
		w.f.Close()
		return err
	}
	name := w.f.Name()
	if err := w.f.Close(); err != nil {
		return err
	}
	w.f = nil
	return w.deps.syncDir(filepath.Dir(name))
}

// File exposes the underlying file. Primarily intended for tests.
func (w *WAL) File() File { return w.f }

func syncDir(path string) error {
	d, err := os.Open(path)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
