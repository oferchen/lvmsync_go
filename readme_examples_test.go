package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/spf13/pflag"
	"go.uber.org/zap"
	"golang.org/x/tools/imports"
	lvmsynccmd "lvmsync_go/cmd/lvmsync"
	"lvmsync_go/internal/config"
)

// TestReadmeExamplesCompile ensures that Go code blocks in README.md compile.
func TestReadmeExamplesCompile(t *testing.T) {
	data, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile("(?s)```go\n(.*?)\n```")
	matches := re.FindAllSubmatch(data, -1)
	for i, m := range matches {
		src := strings.TrimSpace(string(m[1]))
		if strings.HasPrefix(src, "import ") || strings.Contains(src, "device.") {
			continue
		}
		var code []byte
		if strings.HasPrefix(src, "package ") {
			code = []byte(src)
		} else {
			wrapped := "package main\n\nfunc main() {\n" + src + "\n}\n"
			formatted, err := imports.Process("example.go", []byte(wrapped), nil)
			if err != nil {
				t.Fatalf("example %d: %v", i+1, err)
			}
			code = formatted
		}
		dir, err := os.MkdirTemp(".", "example")
		if err != nil {
			t.Fatalf("example %d: %v", i+1, err)
		}
		defer os.RemoveAll(dir)
		file := filepath.Join(dir, "main.go")
		if err := os.WriteFile(file, code, 0o644); err != nil {
			t.Fatalf("example %d: %v", i+1, err)
		}
		cmd := exec.Command("go", "build", file)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("example %d: %v\n%s", i+1, err, out)
		}
	}
}

// TestReadmeShellExamples runs runnable shell blocks from README.md.
func TestReadmeShellExamples(t *testing.T) {
	data, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile("(?s)```sh\n(.*?)\n```")
	matches := re.FindAllSubmatch(data, -1)
	for i, m := range matches {
		block := strings.TrimSpace(string(m[1]))
		lines := strings.Split(block, "\n")
		for j, line := range lines {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "lvmsync") || !strings.Contains(line, "/tmp/src") || !strings.Contains(line, "/tmp/dst") {
				continue
			}
			t.Run(fmt.Sprintf("sh_%d_%d", i, j), func(t *testing.T) {
				if strings.Contains(line, "/tmp/src") {
					if err := os.WriteFile("/tmp/src", nil, 0o644); err != nil {
						t.Fatalf("prep src: %v", err)
					}
				}
				if strings.Contains(line, "/tmp/dst") {
					if err := os.WriteFile("/tmp/dst", nil, 0o644); err != nil {
						t.Fatalf("prep dst: %v", err)
					}
				}
				args := strings.Fields(line)[1:]
				if err := lvmsynccmd.ExecuteWithRunner(args, zap.NewNop(), nil); err != nil {
					t.Fatalf("%s: %v", line, err)
				}
			})
		}
	}
}

// TestFlagDocumentation ensures every flag has a README example.
func TestFlagDocumentation(t *testing.T) {
	data, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatal(err)
	}
	defaults, err := config.DefaultConfig()
	if err != nil {
		t.Fatal(err)
	}
	sets := config.NewFlagSets(defaults)
	var missing []string
	for _, fs := range sets.All() {
		fs.VisitAll(func(f *pflag.Flag) {
			if !strings.Contains(string(data), "--"+f.Name) {
				missing = append(missing, "--"+f.Name)
			}
		})
	}
	if len(missing) > 0 {
		t.Fatalf("flags missing from README: %v", missing)
	}
}
