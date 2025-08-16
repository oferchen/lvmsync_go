package blockio

import (
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
func Open(path string, strict bool) (*File, error) {
	logical, physical := detectBlockSizes(path)
	fd, err := unix.Open(path, unix.O_RDWR|unix.O_CREAT|unix.O_DIRECT, 0)
	direct := err == nil
	var f *os.File
	if direct {
		f = os.NewFile(uintptr(fd), path)
	} else {
		f, err = os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0)
		if err != nil {
			return nil, err
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

// ReaderFrom copies from r into the file, enforcing block alignment.
// If the read size is not aligned and strict mode is disabled, the file falls
// back to buffered I/O and the write proceeds.
func (f *File) ReaderFrom(r io.Reader) (int64, error) {
	bufSize := f.physical
	if bufSize < f.logical {
		bufSize = f.logical
	}
	buf := make([]byte, bufSize)
	var total int64
	for {
		n, err := r.Read(buf)
		if n > 0 {
			if n%f.logical != 0 {
				if f.strict {
					return total, fmt.Errorf("write size %d not multiple of %d", n, f.logical)
				}
				if f.direct {
					if err := f.reopen(); err != nil {
						return total, err
					}
				}
			}
			w, err := f.f.Write(buf[:n])
			total += int64(w)
			if err != nil {
				return total, err
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return total, err
		}
	}
	if f.syncFunc != nil {
		if err := f.syncFunc(); err != nil {
			return total, err
		}
	}
	return total, nil
}

// WriterTo copies from the file to w enforcing block alignment.
func (f *File) WriterTo(w io.Writer) (int64, error) {
	bufSize := f.physical
	if bufSize < f.logical {
		bufSize = f.logical
	}
	buf := make([]byte, bufSize)
	var total int64
	for {
		n, err := f.f.Read(buf)
		if n > 0 {
			if n%f.logical != 0 && f.strict {
				return total, fmt.Errorf("read size %d not multiple of %d", n, f.logical)
			}
			wn, err := w.Write(buf[:n])
			total += int64(wn)
			if err != nil {
				return total, err
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return total, err
		}
	}
	return total, nil
}

func (f *File) reopen() error {
	nf, err := os.OpenFile(f.f.Name(), os.O_RDWR|os.O_CREATE, 0)
	if err != nil {
		return err
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
