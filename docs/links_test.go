package docs

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestMarkdownLinks ensures that internal Markdown links resolve.
func TestMarkdownLinks(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	root := filepath.Dir(wd)
	linkRe := regexp.MustCompile(`\[[^\]]+\]\(([^)]+)\)`)
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".md" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		dir := filepath.Dir(path)
		for _, m := range linkRe.FindAllSubmatch(data, -1) {
			link := string(m[1])
			if i := strings.IndexAny(link, " \t"); i >= 0 {
				link = link[:i]
			}
			if strings.Contains(link, "://") || strings.HasPrefix(link, "mailto:") || strings.HasPrefix(link, "#") {
				continue
			}
			target, frag, _ := strings.Cut(link, "#")
			targetPath := filepath.Join(dir, target)
			if _, err := os.Stat(targetPath); err != nil {
				t.Errorf("%s: broken link %q", path, link)
				continue
			}
			if frag == "" {
				continue
			}
			targetData, err := os.ReadFile(targetPath)
			if err != nil {
				t.Errorf("read %s: %v", targetPath, err)
				continue
			}
			if !anchorExists(targetData, frag) {
				t.Errorf("%s: missing anchor %q in %s", path, frag, targetPath)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
}

func anchorExists(data []byte, frag string) bool {
	frag = strings.ToLower(frag)
	for _, line := range bytes.Split(data, []byte("\n")) {
		if len(line) == 0 || line[0] != '#' {
			continue
		}
		anchor := strings.TrimLeft(string(line), "#")
		anchor = strings.TrimSpace(anchor)
		anchor = strings.ToLower(anchor)
		re := regexp.MustCompile(`[^a-z0-9 -]`)
		anchor = re.ReplaceAllString(anchor, "")
		anchor = strings.ReplaceAll(anchor, " ", "-")
		if anchor == frag {
			return true
		}
	}
	return false
}
