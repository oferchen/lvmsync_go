package config

import (
	"strings"
	"testing"
)

func TestConfigBuilderUnknownKeyWarnings(t *testing.T) {
	cases := []struct {
		name    string
		content string
		keys    []string
	}{
		{
			name:    "single",
			content: "bogus: 1\n",
			keys:    []string{"bogus"},
		},
		{
			name:    "multiple",
			content: "bogus1: 1\nbogus2: 2\n",
			keys:    []string{"bogus1", "bogus2"},
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
			if len(warns) != len(tt.keys) {
				t.Fatalf("expected %d warnings, got %v", len(tt.keys), warns)
			}
			for _, key := range tt.keys {
				found := false
				for _, w := range warns {
					if strings.Contains(w, key) {
						found = true
						break
					}
				}
				if !found {
					t.Fatalf("missing warning for %s: %v", key, warns)
				}
			}
		})
	}
}
