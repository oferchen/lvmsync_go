package lvm

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ParseLVPath splits a logical volume path into its volume group and logical volume names.
// It resolves symlinks before parsing and understands both /dev/<vg>/<lv> and
// /dev/mapper/<vg>-<lv> style paths.
func ParseLVPath(p string) (string, string, error) {
	if p == "" {
		return "", "", fmt.Errorf("empty logical volume path")
	}

	resolved, err := filepath.EvalSymlinks(p)
	if err != nil {
		return "", "", fmt.Errorf("resolve %s: %w", p, err)
	}

	resolved = filepath.Clean(resolved)
	sep := string(filepath.Separator)
	if strings.Contains(resolved, sep+"mapper"+sep) {
		base := filepath.Base(resolved)
		replaced := strings.ReplaceAll(base, "--", "\x00")
		idx := strings.Index(replaced, "-")
		if idx < 0 {
			return "", "", fmt.Errorf("invalid logical volume path: %s", p)
		}
		vg := strings.ReplaceAll(replaced[:idx], "\x00", "-")
		lv := strings.ReplaceAll(replaced[idx+1:], "\x00", "-")
		if vg == "" || lv == "" {
			return "", "", fmt.Errorf("invalid logical volume path: %s", p)
		}
		return vg, lv, nil
	}

	vg := filepath.Base(filepath.Dir(resolved))
	lv := filepath.Base(resolved)
	if vg == "" || lv == "" {
		return "", "", fmt.Errorf("invalid logical volume path: %s", p)
	}
	return vg, lv, nil
}
