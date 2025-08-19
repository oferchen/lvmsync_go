package main

import (
	"os"

	cfg "lvmsync_go/internal/config"
)

func main() {
	defaults, err := cfg.DefaultConfig()
	if err != nil {
		panic(err)
	}
	doc := cfg.EnvDoc(cfg.NewFlagSets(defaults))
	if err := os.WriteFile("docs/env.md", []byte(doc), 0o644); err != nil {
		panic(err)
	}
}
