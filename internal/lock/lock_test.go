package lock

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAcquireRelease(t *testing.T) {
	dir := t.TempDir()
	restore := SetBaseDir(dir)
	defer restore()
	l, err := Acquire("vg0", "lv0")
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	path := filepath.Join(dir, "vg0.lv0.lock")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("lock file missing: %v", err)
	}
	if err := l.Release(); err != nil {
		t.Fatalf("Release: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("lock file not removed")
	}
}

func TestAcquireValidIdentifiers(t *testing.T) {
	dir := t.TempDir()
	restore := SetBaseDir(dir)
	defer restore()
	cases := []struct{ vg, lv string }{
		{"vg-1", "lv_1"},
		{"vg.name", "lv.name"},
	}
	for _, c := range cases {
		t.Run(c.vg+"/"+c.lv, func(t *testing.T) {
			l, err := Acquire(c.vg, c.lv)
			if err != nil {
				t.Fatalf("Acquire: %v", err)
			}
			if err := l.Release(); err != nil {
				t.Fatalf("Release: %v", err)
			}
		})
	}
}

func TestAcquireInvalidIdentifiers(t *testing.T) {
	dir := t.TempDir()
	restore := SetBaseDir(dir)
	defer restore()
	cases := []struct{ vg, lv string }{
		{"", "lv"},
		{"vg", ""},
		{"vg/1", "lv"},
		{"vg", "lv/1"},
		{"vg!", "lv"},
	}
	for _, c := range cases {
		if _, err := Acquire(c.vg, c.lv); err == nil {
			t.Fatalf("Acquire(%q,%q) succeeded; want error", c.vg, c.lv)
		}
	}
}
