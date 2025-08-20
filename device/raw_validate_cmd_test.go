package device

import (
	"os/exec"
	"path/filepath"
	"testing"
)

func TestValidateCmd(t *testing.T) {
	truePath, err := exec.LookPath("true")
	if err != nil {
		t.Fatalf("missing true binary: %v", err)
	}
	invalidBase := filepath.Join(filepath.Dir(truePath), "tr ue")
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
			name:    "relative command path",
			path:    "true",
			wantErr: "command path must be absolute",
		},
		{
			name:    "nul in path",
			path:    "/true\x00",
			wantErr: "command path contains NUL byte",
		},
		{
			name:    "nul in arg",
			path:    truePath,
			args:    []string{"foo\x00"},
			wantErr: "command argument contains NUL byte",
		},
		{
			name:    "invalid characters in basename",
			path:    invalidBase,
			wantErr: "command path " + invalidBase + " contains invalid characters",
		},
		{
			name:    "nonexistent command",
			path:    "/does-not-exist",
			wantErr: "/does-not-exist: exec: \"/does-not-exist\": stat /does-not-exist: no such file or directory",
		},
		{
			name: "valid absolute path",
			path: truePath,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateCmd(tc.path, tc.args)
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
