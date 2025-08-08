package main

import (
	"errors"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type failCore struct{}

func (failCore) Enabled(zapcore.Level) bool        { return false }
func (failCore) With([]zapcore.Field) zapcore.Core { return failCore{} }
func (failCore) Check(zapcore.Entry, *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	return nil
}
func (failCore) Write(zapcore.Entry, []zapcore.Field) error { return nil }
func (failCore) Sync() error                                { return errors.New("sync failure") }

func TestSyncAndExitLogsError(t *testing.T) {
	logger := zap.New(failCore{})

	oldExit := exitFunc
	defer func() { exitFunc = oldExit }()

	var code int
	exitFunc = func(c int) { code = c }

	syncAndExit(logger)
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
}
