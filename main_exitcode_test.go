package main

import (
	"fmt"
	"testing"

	"go.uber.org/zap"

	rootcmd "github.com/oferchen/lvmsync_go/cmd/root"
	"github.com/oferchen/lvmsync_go/internal/config"
	"github.com/oferchen/lvmsync_go/internal/exitcode"
)

func TestRunnerExitCodes(t *testing.T) {
	cases := []struct {
		name   string
		goos   string
		cfgErr error
		runErr error
		want   int
	}{
		{"success", "linux", nil, nil, exitcode.OK},
		{"platform", "darwin", nil, nil, 1},
		{"capability", "linux", fmt.Errorf("privilege: %w", exitcode.ErrCapability), nil, exitcode.Capability},
		{"config", "linux", fmt.Errorf("config invalid: %w", exitcode.ErrConfig), nil, exitcode.Config},
		{"runtime", "linux", nil, fmt.Errorf("boom"), 1},
		{"verify", "linux", nil, fmt.Errorf("digest mismatch: %w", exitcode.ErrVerify), exitcode.Verify},
		{"precondition", "linux", nil, fmt.Errorf("precondition: %w", exitcode.ErrPrecondition), exitcode.Precondition},
		{"resumable", "linux", nil, fmt.Errorf("resumable: %w", exitcode.ErrResumable), exitcode.Resumable},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			var code int
			r := NewRunnerWithDeps(
				func() (*config.Config, []string, *zap.Logger, error) {
					if tt.cfgErr != nil {
						return nil, nil, nil, tt.cfgErr
					}
					return &config.Config{}, nil, zap.NewNop(), nil
				},
				func(_ *config.Config, _ []string, _ *zap.Logger) error { return tt.runErr },
				rootcmd.SyncLogger,
				func(c int) { code = c },
				func() *zap.Logger { return zap.NewNop() },
				tt.goos,
			)
			r.Run()
			if code != tt.want {
				t.Fatalf("expected exit code %d, got %d", tt.want, code)
			}
		})
	}
}

func TestExitCodeValues(t *testing.T) {
	cases := []struct {
		name string
		code int
		want int
	}{
		{"OK", exitcode.OK, 0},
		{"Precondition", exitcode.Precondition, 2},
		{"Verify", exitcode.Verify, 3},
		{"Resumable", exitcode.Resumable, 4},
		{"Config", exitcode.Config, 5},
		{"Capability", exitcode.Capability, 6},
	}
	for _, tt := range cases {
		if tt.code != tt.want {
			t.Errorf("exitcode.%s = %d, want %d", tt.name, tt.code, tt.want)
		}
	}
}
