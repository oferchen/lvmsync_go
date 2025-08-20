package transfer

import (
	"fmt"
	"io"
	"os"
)

// Deps bundles external interactions for transfer package components.
type Deps struct {
	CreateStateFile    func(string) (io.WriteCloser, error)
	DetectBestStrategy func() string
	FdatasyncFile      func(*os.File) error
	PunchHole          func(*os.File, uint64, int) error
}

func defaultCreateStateFile(name string) (io.WriteCloser, error) {
	f, err := os.Create(name)
	if err != nil {
		return nil, fmt.Errorf("create state file: %w", err)
	}
	if err := f.Chmod(0o600); err != nil {
		f.Close()
		return nil, fmt.Errorf("chmod state file: %w", err)
	}
	return f, nil
}

func detectBestStrategy() string {
	if supportsChecksumAcceleration() {
		return StrategyChecksum
	}
	return StrategyRollingHash
}

// DefaultDeps provides real implementations for production use.
var DefaultDeps = &Deps{
	CreateStateFile:    defaultCreateStateFile,
	DetectBestStrategy: detectBestStrategy,
	FdatasyncFile:      fdatasync,
	PunchHole:          punchHole,
}
