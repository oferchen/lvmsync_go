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
	"lvmsync_go/dedup"
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
			if line == "" || strings.HasPrefix(line, "#") || strings.ContainsAny(line, "[]<>") || !strings.Contains(line, "lvmsync") || strings.Contains(line, "lvmsyncd") {
				continue
			}
			fields := strings.Fields(line)
			idx := -1
			for k, f := range fields {
				if f == "lvmsync" {
					idx = k
					break
				}
			}
			if idx == -1 {
				continue
			}
			// ensure anything before lvmsync is an env assignment
			for _, f := range fields[:idx] {
				if !strings.Contains(f, "=") {
					idx = -1
					break
				}
			}
			if idx == -1 {
				continue
			}
			args := append([]string{}, fields[idx+1:]...)
			if len(args) == 0 {
				continue
			}
			sub := args[0]
			if sub != "run" {
				continue
			}
			allowed := map[string]struct{}{
				"--dry-run":        {},
				"--verify-only":    {},
				"--plan":           {},
				"--transport":      {},
				"--allow-insecure": {},
				"--delta":          {},
				"--dedup-strategy": {},
				"--compress":       {},
			}
			valid := false
			for _, a := range args[1:] {
				if strings.HasPrefix(a, "--") {
					flag := a
					if i := strings.Index(flag, "="); i >= 0 {
						flag = flag[:i]
					}
					if _, ok := allowed[flag]; !ok {
						valid = false
						break
					}
					if flag == "--verify-only" || flag == "--plan" || flag == "--transport" {
						valid = true
					}
				}
			}
			if !valid {
				continue
			}

			srcIdx, dstIdx := -1, -1
			for k := len(args) - 1; k >= 0; k-- {
				a := args[k]
				if strings.HasPrefix(a, "-") {
					continue
				}
				switch a {
				case "run", "verify", "manifest", "rebuild", "plan":
					continue
				}
				if dstIdx == -1 {
					dstIdx = k
					continue
				}
				srcIdx = k
				break
			}
			if srcIdx == -1 || dstIdx == -1 {
				continue
			}
			src, dst := args[srcIdx], args[dstIdx]
			if strings.Contains(src, ":") && !strings.Contains(dst, ":") {
				src, dst = dst, src
				args[srcIdx], args[dstIdx] = src, dst
			}
			if strings.Contains(src, ":") {
				continue
			}
			args[srcIdx] = ensureTempPath(src)
			if !strings.Contains(dst, ":") {
				args[dstIdx] = ensureTempPath(dst)
			}

			for k := 0; k < len(args); k++ {
				a := args[k]
				if !strings.HasPrefix(a, "--") {
					continue
				}
				if strings.Contains(a, "=") {
					parts := strings.SplitN(a, "=", 2)
					if strings.HasPrefix(parts[1], "/") && !strings.Contains(parts[1], ":") {
						parts[1] = ensureTempPath(parts[1])
						args[k] = parts[0] + "=" + parts[1]
					}
				} else if k+1 < len(args) && strings.HasPrefix(args[k+1], "/") && !strings.Contains(args[k+1], ":") {
					args[k+1] = ensureTempPath(args[k+1])
				}
			}

			dryRun := false
			for _, a := range args {
				if a == "--dry-run" || strings.HasPrefix(a, "--dry-run=") {
					dryRun = true
					break
				}
			}
			if !dryRun {
				args = append(args, "--dry-run")
			}

			t.Run(fmt.Sprintf("sh_%d_%d", i, j), func(t *testing.T) {
				if err := lvmsynccmd.ExecuteWithRunner(args, zap.NewNop(), nil); err != nil {
					t.Fatalf("%s: %v", line, err)
				}
			})
		}
	}
}

func ensureTempPath(p string) string {
	if strings.Contains(p, ":") {
		return p
	}
	if strings.HasPrefix(p, "/tmp/") {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err == nil {
			_ = os.WriteFile(p, nil, 0o644)
		}
		return p
	}
	f, err := os.CreateTemp("", "lvmsync")
	if err != nil {
		return p
	}
	f.Close()
	return f.Name()
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

func TestDedupStrategyDocs(t *testing.T) {
	want := dedup.AutoStrategyTable()
	data, err := os.ReadFile("docs/dedup_strategies.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), want) {
		t.Fatalf("dedup strategy table out of sync")
	}
}
