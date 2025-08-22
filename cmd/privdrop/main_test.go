package main

import (
	"errors"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	"github.com/oferchen/lvmsync_go/escalate"
)

func TestRunnerRun(t *testing.T) {
	tcs := []struct {
		name     string
		ensure   func(escalate.Options, *zap.Logger) (bool, error)
		drop     func(escalate.Options, *zap.Logger) error
		exitCode int
		logMsg   string
		logLevel zapcore.Level
	}{
		{
			name: "ensure_error",
			ensure: func(escalate.Options, *zap.Logger) (bool, error) {
				return false, errors.New("boom")
			},
			drop: func(escalate.Options, *zap.Logger) error {
				t.Fatalf("drop should not be called")
				return nil
			},
			exitCode: 1,
			logMsg:   "ensure_root_or_reexec",
			logLevel: zapcore.ErrorLevel,
		},
		{
			name: "reexeced",
			ensure: func(escalate.Options, *zap.Logger) (bool, error) {
				return true, nil
			},
			drop: func(escalate.Options, *zap.Logger) error {
				t.Fatalf("drop should not be called")
				return nil
			},
			exitCode: 0,
			logMsg:   "",
		},
		{
			name: "drop_error",
			ensure: func(escalate.Options, *zap.Logger) (bool, error) {
				return false, nil
			},
			drop: func(escalate.Options, *zap.Logger) error {
				return errors.New("nope")
			},
			exitCode: 1,
			logMsg:   "drop_to_invoker_if_sudo",
			logLevel: zapcore.ErrorLevel,
		},
		{
			name: "success",
			ensure: func(escalate.Options, *zap.Logger) (bool, error) {
				return false, nil
			},
			drop: func(escalate.Options, *zap.Logger) error {
				return nil
			},
			exitCode: 0,
			logMsg:   "running_unprivileged",
			logLevel: zapcore.InfoLevel,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			r := newRunnerWithDeps(tc.ensure, tc.drop)
			core, logs := observer.New(zapcore.DebugLevel)
			logger := zap.New(core)

			code := r.run(logger)
			if code != tc.exitCode {
				t.Fatalf("exit code: got %d want %d", code, tc.exitCode)
			}

			entries := logs.All()
			if tc.logMsg == "" {
				if len(entries) != 0 {
					t.Fatalf("expected no logs, got %v", entries)
				}
				return
			}
			if len(entries) != 1 {
				t.Fatalf("expected 1 log, got %d", len(entries))
			}
			if entries[0].Message != tc.logMsg {
				t.Fatalf("log message: got %q want %q", entries[0].Message, tc.logMsg)
			}
			if entries[0].Level != tc.logLevel {
				t.Fatalf("log level: got %v want %v", entries[0].Level, tc.logLevel)
			}
		})
	}
}
