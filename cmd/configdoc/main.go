package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	cfg "lvmsync_go/internal/config"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

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
