package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"golang.org/x/tools/imports"
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
