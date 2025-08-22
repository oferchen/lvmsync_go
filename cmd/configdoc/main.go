package main

import (
	"errors"
	"os"
	"path/filepath"

	"go.uber.org/zap"
	rootcmd "github.com/oferchen/lvmsync_go/cmd/root"
	cfg "github.com/oferchen/lvmsync_go/internal/config"
)

type runner struct {
	run        func() error
	syncLogger func(*zap.Logger)
	exit       func(int)
	newLogger  func() *zap.Logger
}

func newRunner() *runner {
	return &runner{
		run:        run,
		syncLogger: rootcmd.SyncLogger,
		exit:       os.Exit,
		newLogger:  func() *zap.Logger { return zap.NewExample() },
	}
}

func newRunnerWithDeps(runFunc func() error, syncFunc func(*zap.Logger), exitFunc func(int), loggerFunc func() *zap.Logger) *runner {
	r := newRunner()
	if runFunc != nil {
		r.run = runFunc
	}
	if syncFunc != nil {
		r.syncLogger = syncFunc
	}
	if exitFunc != nil {
		r.exit = exitFunc
	}
	if loggerFunc != nil {
		r.newLogger = loggerFunc
	}
	return r
}

func (r *runner) Run() {
	logger := r.newLogger()
	defer r.syncLogger(logger)
	if err := r.run(); err != nil {
		logger.Error("run_failed", zap.Error(err))
		r.exit(1)
		return
	}
	r.exit(0)
}

func main() { newRunner().Run() }

func run() error {
	defaults, err := cfg.DefaultConfig()
	if err != nil {
		return err
	}
	doc := cfg.EnvDocHeader + cfg.EnvDoc(cfg.NewFlagSets(defaults))
	path, err := outputPath()
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(doc), 0o644); err != nil {
		return err
	}
	return nil
}

func outputPath() (string, error) {
	root, err := findRepoRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "docs", "config_env.md"), nil
}

func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("go.mod not found")
		}
		dir = parent
	}
}
