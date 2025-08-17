package config

import "testing"

func TestConfigBuilderAliasKeysNoWarnings(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{
			name:    "hyphen_alias",
			content: "numa-pin: true\n",
		},
		{
			name:    "underscore_alias",
			content: "fs_freeze_command: /bin/true\nfs_thaw_command: /bin/true\n",
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			cfgPath := writeTempConfig(t, tt.content)
			defaults, err := DefaultConfig()
			if err != nil {
				t.Fatalf("DefaultConfig: %v", err)
			}
			builder := NewBuilder(defaults)
			fs, args := newFlagSet([]string{"--config", cfgPath})
			_, _, warns, err := builder.Build(fs, args)
			if err != nil {
				t.Fatalf("Build: %v", err)
			}
			if len(warns) != 0 {
				t.Fatalf("expected no warnings, got %v", warns)
			}
		})
	}
}
