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
	type envCase struct {
		flag  string
		env   string
		val   string
		slice bool
	}
	cases := map[string][]envCase{
		"General Options":       {{flag: "source-type", env: "LVMSYNC_SOURCE_TYPE", val: "raw"}},
		"SSH Options":           {{flag: "ssh-host", env: "LVMSYNC_SSH_HOST", val: "ssh.example"}},
		"Remote Options":        {{flag: "lvmsync-path", env: "LVMSYNC_LVMSYNC_PATH", val: "/bin/lvmsync"}},
		"Deduplication Options": {{flag: "dedup-strategy", env: "LVMSYNC_DEDUP_STRATEGY", val: "bloom"}},
		"Compression Options":   {{flag: "compress", env: "LVMSYNC_COMPRESSION_COMPRESS", val: "zstd"}},
		"LVM Options": {
			{flag: "lvm-escalation", env: "LVMSYNC_LVM_ESCALATION", val: "su"},
			{flag: "target-vgs", env: "LVMSYNC_TARGET_VGS", val: "vg1", slice: true},
		},
		"Transport Options": {{flag: "transport", env: "LVMSYNC_TRANSPORT_TRANSPORT", val: "ssh"}},
		"Manifest Options":  {{flag: "manifest-path", env: "LVMSYNC_MANIFEST_PATH", val: "/tmp/manifest"}},
	}
	for _, fs := range flagSets.All() {
		tc, ok := cases[fs.Name()]
		if !ok {
			t.Fatalf("missing case for %q", fs.Name())
		}
		for _, c := range tc {
			t.Setenv(c.env, c.val)
		}
	}
	v, _, _, _, err := buildViper(flagSets)
	if err != nil {
		t.Fatalf("buildViper: %v", err)
	}
	for _, fs := range flagSets.All() {
		for _, c := range cases[fs.Name()] {
			if c.slice {
				got := v.GetStringSlice(c.flag)
				if len(got) != 1 || got[0] != c.val {
					t.Errorf("%s %s=%v want [%s]", fs.Name(), c.flag, got, c.val)
				}
				continue
			}
			if got := v.GetString(c.flag); got != c.val {
				t.Errorf("%s %s=%q want %q", fs.Name(), c.flag, got, c.val)
			}
		}
	}
}
