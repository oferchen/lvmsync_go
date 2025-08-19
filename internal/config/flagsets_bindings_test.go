package config

import (
	"testing"
)

func TestFlagSetEnvBindings(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	defaults, err := DefaultConfig()
	if err != nil {
		t.Fatalf("default config: %v", err)
	}
	flagSets := NewFlagSets(defaults)
	cases := map[string]struct {
		flag string
		env  string
		val  string
	}{
		"General Options":       {flag: "source-type", env: "LVMSYNC_SOURCE_TYPE", val: "raw"},
		"SSH Options":           {flag: "ssh-host", env: "LVMSYNC_SSH_HOST", val: "ssh.example"},
		"Remote Options":        {flag: "lvmsync-path", env: "LVMSYNC_LVMSYNC_PATH", val: "/bin/lvmsync"},
		"Deduplication Options": {flag: "dedup-strategy", env: "LVMSYNC_DEDUP_STRATEGY", val: "bloom"},
		"Compression Options":   {flag: "compress", env: "LVMSYNC_COMPRESSION_COMPRESS", val: "zstd"},
		"LVM Options":           {flag: "lvm-escalation", env: "LVMSYNC_LVM_ESCALATION", val: "su"},
		"Transport Options":     {flag: "transport", env: "LVMSYNC_TRANSPORT_TRANSPORT", val: "ssh"},
		"Manifest Options":      {flag: "manifest-path", env: "LVMSYNC_MANIFEST_PATH", val: "/tmp/manifest"},
	}
	for _, fs := range flagSets.All() {
		tc, ok := cases[fs.Name()]
		if !ok {
			t.Fatalf("missing case for %q", fs.Name())
		}
		t.Setenv(tc.env, tc.val)
	}
	v, _, _, _, err := buildViper(flagSets)
	if err != nil {
		t.Fatalf("buildViper: %v", err)
	}
	for _, fs := range flagSets.All() {
		tc := cases[fs.Name()]
		if got := v.GetString(tc.flag); got != tc.val {
			t.Errorf("%s %s=%q want %q", fs.Name(), tc.flag, got, tc.val)
		}
	}
}
