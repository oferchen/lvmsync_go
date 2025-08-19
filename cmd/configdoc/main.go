package main

import (
	"os"
	"path/filepath"

	cfg "lvmsync_go/internal/config"
)

func main() {
	defaults, err := cfg.DefaultConfig()
	if err != nil {
		panic(err)
	}
	doc := cfg.EnvDocHeader + cfg.EnvDoc(cfg.NewFlagSets(defaults))
	if err := os.WriteFile(outputPath(), []byte(doc), 0o644); err != nil {
		panic(err)
	}
}

func outputPath() string {
	root := findRepoRoot()
	return filepath.Join(root, "docs", "config_env.md")
}

func findRepoRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			panic("go.mod not found")
		}
		dir = parent
	}
}
