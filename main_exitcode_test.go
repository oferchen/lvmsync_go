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
		{"platform", "darwin", nil, nil, exitcode.ErrPlatform},
		{"capability", "linux", errors.New("privilege check failed: missing"), nil, exitcode.ErrCapability},
		{"config", "linux", errors.New("cfg"), nil, exitcode.ErrConfig},
		{"device", "linux", nil, errors.New("device gone"), exitcode.ErrDevice},
		{"runtime", "linux", nil, errors.New("boom"), exitcode.ErrRuntime},
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
