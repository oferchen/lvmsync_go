package main

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"

	"lvmsync_go/internal/exitcode"
	"lvmsync_go/transport"
)

func TestRunLoggerError(t *testing.T) {
	code := run(func() (*zap.Logger, error) { return nil, errors.New("fail") }, NewRunner())
	if code != exitcode.ErrConfig {
		t.Fatalf("expected %d got %d", exitcode.ErrConfig, code)
	}
}

func TestRunExecuteError(t *testing.T) {
	r := NewRunnerWithDeps(func(context.Context, Options, *zap.Logger, func(string, transport.Config) (transport.Interface, error)) error {
		return errors.New("boom")
	}, nil)
	code := run(func() (*zap.Logger, error) { return zap.NewNop(), nil }, r)
	if code != exitcode.ErrRuntime {
		t.Fatalf("expected %d got %d", exitcode.ErrRuntime, code)
	}
}
