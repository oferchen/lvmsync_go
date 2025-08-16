package blockio

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

// File represents an open file with block size metadata.
type File struct {
	f        *os.File
	logical  int
	physical int
	direct   bool
	strict   bool
	syncFunc func() error
}

// Open opens path with O_DIRECT when possible and records queue block sizes.
// When strict is false and a later write is not block aligned, the file
// reopens without O_DIRECT to allow buffered I/O.
func Open(path string, direct, strict bool) (*File, error) {
	var err error
	if _, err = os.Stat(path); err != nil {
		return nil, fmt.Errorf("stat %q: %w", path, err)
	}
	logical, physical := detectBlockSizes(path)
	var fd int
	if direct {
		fd, err = unix.Open(path, unix.O_RDWR|unix.O_DIRECT, 0)
		if err != nil {
			if errors.Is(err, unix.EINVAL) && !strict {
				direct = false
			} else {
				return nil, fmt.Errorf("open %q: %w", path, err)
			}
		}
	}
	var f *os.File
	if direct && err == nil {
		f = os.NewFile(uintptr(fd), path)
	} else {
		f, err = os.OpenFile(path, os.O_RDWR, 0)
		if err != nil {
			return nil, fmt.Errorf("open %q: %w", path, err)
		}
	}
	bf := &File{f: f, logical: logical, physical: physical, direct: direct, strict: strict}
	bf.syncFunc = f.Sync
	return bf, nil
}

// Close closes the underlying file.
func (f *File) Close() error { return f.f.Close() }

// Name returns the underlying file name.
func (f *File) Name() string { return f.f.Name() }

// Logical returns the logical block size.
func (f *File) Logical() int { return f.logical }

// Physical returns the physical block size.
func (f *File) Physical() int { return f.physical }

// Direct reports whether O_DIRECT is currently enabled.
func (f *File) Direct() bool { return f.direct }

func (f *File) Write(p []byte) (int, error) {
	n := len(p)
	if n%f.logical != 0 || n%f.physical != 0 {
		if f.strict {
			return 0, fmt.Errorf("write size %d not multiple of %d or %d", n, f.logical, f.physical)
		}
		if f.direct {
			if err := f.reopen(); err != nil {
				return 0, err
			}
		}
	}
	return f.f.Write(p)
}

func (f *File) Read(p []byte) (int, error) {
	n, err := f.f.Read(p)
	if n%f.logical != 0 && f.strict {
		return n, fmt.Errorf("read size %d not multiple of %d", n, f.logical)
	}
	return n, err
}

func (f *File) Seek(offset int64, whence int) (int64, error) { return f.f.Seek(offset, whence) }

// ReadAt reads from the file at a specific offset.
func (f *File) ReadAt(p []byte, off int64) (int, error) {
	n, err := f.f.ReadAt(p, off)
	if n%f.logical != 0 && f.strict {
		return n, fmt.Errorf("read size %d not multiple of %d", n, f.logical)
	}
	return n, err
}

// WriteAt writes to the file at a specific offset, falling back to buffered
// I/O when misaligned and strict mode is disabled.
func (f *File) WriteAt(p []byte, off int64) (int, error) {
	n := len(p)
	if n%f.logical != 0 || n%f.physical != 0 {
		if f.strict {
			return 0, fmt.Errorf("write size %d not multiple of %d or %d", n, f.logical, f.physical)
		}
		if f.direct {
			if err := f.reopen(); err != nil {
				return 0, err
			}
		}
	}
	return f.f.WriteAt(p, off)
}

// Size returns the current size of the underlying file.
func (f *File) Size() int64 {
	if fi, err := f.f.Stat(); err == nil {
		return fi.Size()
	}
	off, _ := f.f.Seek(0, io.SeekEnd)
	return off
}

func (f *File) Sync() error {
	if f.syncFunc != nil {
		return f.syncFunc()
	}
	return nil
}

func (f *File) reopen() error {
	nf, err := os.OpenFile(f.f.Name(), os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("open %q: %w", f.f.Name(), err)
	}
	old := f.f
	f.f = nf
	f.direct = false
	f.syncFunc = nf.Sync
	return old.Close()
}

func detectBlockSizes(devicePath string) (logical, physical int) {
	fi, err := os.Stat(devicePath)
	if err != nil {
		return 4096, 4096
	}
	stat, ok := fi.Sys().(*unix.Stat_t)
	if !ok {
		stat = &unix.Stat_t{}
		if err := unix.Stat(devicePath, stat); err != nil {
			return 4096, 4096
		}
	}
	rdev := uint64(stat.Rdev)
	queue := filepath.Join("/sys/dev/block", fmt.Sprintf("%d:%d/queue", unix.Major(rdev), unix.Minor(rdev)))
	logical = readInt(queue, "logical_block_size")
	physical = readInt(queue, "physical_block_size")
	if logical <= 0 {
		logical = 4096
	}
	if physical <= 0 {
		physical = logical
	}
	return logical, physical
}

func readInt(dir, name string) int {
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return 0
	}
	v, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0
	}
	return v
}
