package verify

import (
	"os"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestRunLogsMismatchBlock(t *testing.T) {
	dir := t.TempDir()
	src := dir + "/src"
	dst := dir + "/dst"
	if err := os.WriteFile(src, []byte("foo"), 0o600); err != nil {
		t.Fatalf("write src: %v", err)
	}
	if err := os.WriteFile(dst, []byte("bar"), 0o600); err != nil {
		t.Fatalf("write dst: %v", err)
	}
	core, logs := observer.New(zapcore.InfoLevel)
	logger := zap.New(core)
	err := Run([]string{src, dst}, logger)
	if err == nil {
		t.Fatalf("expected error")
	}
	if logs.FilterMessage("mismatched_block").Len() == 0 {
		t.Fatalf("expected mismatched_block log, got %v", logs.All())
	}
}
