package device

import (
	"errors"
	"fmt"
	"os/exec"
	"time"
)

var (
	blkidPath string
	blkidErr  error
	lvsPath   string
	lvsErr    error
	lsblkPath string
	lsblkErr  error
)

const identityTimeout = 5 * time.Second

// ErrDependencyMissing indicates that a required external command is unavailable.
var ErrDependencyMissing = errors.New("dependency missing")

func init() {
	var err error
	if blkidPath, err = exec.LookPath("blkid"); err != nil {
		blkidErr = fmt.Errorf("blkid: %w", ErrDependencyMissing)
	}
	if lvsPath, err = exec.LookPath("lvs"); err != nil {
		lvsErr = fmt.Errorf("lvs: %w", ErrDependencyMissing)
	}
	if lsblkPath, err = exec.LookPath("lsblk"); err != nil {
		lsblkErr = fmt.Errorf("lsblk: %w", ErrDependencyMissing)
	}
}
