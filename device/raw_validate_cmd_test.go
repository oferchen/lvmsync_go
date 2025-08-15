package device

import (
	"testing"
)

func TestValidateCmd(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		args    []string
		wantErr string
	}{
		{
			name:    "empty path",
			path:    "",
			wantErr: "command path is empty",
		},
		{
			name:    "nul in path",
			path:    "true\x00",
			wantErr: "command path contains NUL byte",
		},
		{
			name:    "nul in arg",
			path:    "true",
			args:    []string{"foo\x00"},
			wantErr: "command argument contains NUL byte",
		},
		{
			name:    "nonexistent command",
			path:    "does-not-exist",
			wantErr: "does-not-exist: exec: \"does-not-exist\": executable file not found in $PATH",
		},
		{
			name: "valid command",
			path: "true",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateCmd(tc.path, tc.args)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error %q", tc.wantErr)
			}
			if err.Error() != tc.wantErr {
				t.Fatalf("got %q, want %q", err.Error(), tc.wantErr)
			}
		})
	}
}
