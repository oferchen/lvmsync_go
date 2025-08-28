package root

import (
	"context"
	"errors"
	"testing"

	"github.com/oferchen/lvmsync_go/internal/exitcode"
)

func TestExitCode(t *testing.T) {
	cases := []struct {
		name string
		err  error
		code int
	}{
		{"nil", nil, exitcode.OK},
		{"capability", exitcode.ErrCapability, exitcode.Capability},
		{"config", exitcode.ErrConfig, exitcode.Config},
		{"precondition", exitcode.ErrPrecondition, exitcode.Precondition},
		{"resumable", exitcode.ErrResumable, exitcode.Resumable},
		{"canceled", context.Canceled, exitcode.Resumable},
		{"verify", exitcode.ErrVerify, exitcode.Verify},
		{"other", errors.New("boom"), 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if c := ExitCode(tc.err); c != tc.code {
				t.Fatalf("ExitCode(%v)=%d want %d", tc.err, c, tc.code)
			}
		})
	}
}
