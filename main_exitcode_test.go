package main

import (
	"errors"
	"testing"

	"go.uber.org/zap"

	rootcmd "lvmsync_go/cmd/root"
	"lvmsync_go/internal/config"
	"lvmsync_go/internal/exitcode"
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
		{"platform", "darwin", nil, nil, exitcode.ErrPlatform},
		{"capability", "linux", errors.New("privilege check failed: missing"), nil, exitcode.ErrCapability},
		{"config", "linux", errors.New("config invalid"), nil, exitcode.ErrConfig},
		{"device", "linux", nil, errors.New("device gone"), exitcode.ErrDevice},
		{"runtime", "linux", nil, errors.New("boom"), exitcode.ErrRuntime},
		{"verify", "linux", nil, errors.New("digest mismatch"), exitcode.ErrVerify},
		{"partial", "linux", nil, errors.New("received signal: interrupt"), exitcode.ErrPartial},
		{"precondition", "linux", nil, errors.New("precondition not met"), exitcode.ErrPrecondition},
		{"resumable", "linux", nil, errors.New("resumable: retry"), exitcode.ErrResumable},
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
		{"ErrCapability", exitcode.ErrCapability, 10},
		{"ErrDevice", exitcode.ErrDevice, 20},
		{"ErrPlatform", exitcode.ErrPlatform, 30},
		{"ErrConfig", exitcode.ErrConfig, 40},
		{"ErrRuntime", exitcode.ErrRuntime, 50},
		{"ErrVerify", exitcode.ErrVerify, 60},
		{"ErrPartial", exitcode.ErrPartial, 70},
		{"ErrPrecondition", exitcode.ErrPrecondition, 80},
		{"ErrResumable", exitcode.ErrResumable, 90},
	}
	for _, tt := range cases {
		if tt.code != tt.want {
			t.Errorf("exitcode.%s = %d, want %d", tt.name, tt.code, tt.want)
		}
	}
}
