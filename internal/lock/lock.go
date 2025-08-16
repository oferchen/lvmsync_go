// Package lock provides lightweight file locking for volume group and logical
// volume operations. VG and LV identifiers must match the regular expression
// `^[A-Za-z0-9_.-]+$` to avoid path traversal and other unsafe names.
package lock

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"golang.org/x/sys/unix"
)

var baseDir = "/run/lvmsync"

var identRe = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

// SetBaseDir overrides the default lock directory for tests.
func SetBaseDir(dir string) func() {
	orig := baseDir
	baseDir = dir
	return func() { baseDir = orig }
}

// Lock represents an acquired lock file.
type Lock struct {
	f *os.File
}

// Acquire obtains an exclusive lock for the given volume group and logical volume.
// The lock file is placed at /run/lvmsync/<vg>.<lv>.lock.
func Acquire(vg, lv string) (*Lock, error) {
	if !identRe.MatchString(vg) || !identRe.MatchString(lv) {
		return nil, fmt.Errorf("invalid vg/lv")
	}
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(baseDir, fmt.Sprintf("%s.%s.lock", vg, lv))
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		f.Close()
		return nil, err
	}
	return &Lock{f: f}, nil
}

// Release unlocks and removes the underlying lock file.
func (l *Lock) Release() error {
	if l == nil || l.f == nil {
		return nil
	}
	defer func() { l.f = nil }()
	if err := unix.Flock(int(l.f.Fd()), unix.LOCK_UN); err != nil {
		_ = l.f.Close()
		return err
	}
	path := l.f.Name()
	if err := l.f.Close(); err != nil {
		return err
	}
	return os.Remove(path)
}
