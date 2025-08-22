package lvmsync

import (
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"go.uber.org/zap"
)

func TestRunCommandProbeOnlyOutputsIdentityTuple(t *testing.T) {
	// Probe-only output format:
	// size_bytes kernel_uuid gpt_uuid mbr_signature fs_uuid major minor manifest_epoch
	src := t.TempDir() + "/src"
	if err := os.WriteFile(src, []byte("data"), 0o600); err != nil {
		t.Fatalf("write src: %v", err)
	}
	dst := t.TempDir() + "/dst"

	fi, err := os.Stat(src)
	if err != nil {
		t.Fatalf("stat src: %v", err)
	}

	r := NewRunnerWithDeps(func(srcArg, dstArg string, _ RunOptions, _ *zap.Logger) error {
		_, err := fmt.Fprintf(os.Stdout, "%d k g  f 1 2 3\n", fi.Size())
		return err
	}, nil, nil)

	oldStdout := os.Stdout
	rPipe, wPipe, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = wPipe

	if err := ExecuteWithRunner([]string{"run", "--probe-only", src, dst}, zap.NewNop(), r); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	wPipe.Close()
	os.Stdout = oldStdout
	out, err := io.ReadAll(rPipe)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	got := strings.TrimSpace(string(out))
	exp := fmt.Sprintf("%d k g  f 1 2 3", fi.Size())
	if got != exp {
		t.Fatalf("expected %q, got %q", exp, got)
	}
	parts := strings.Split(got, " ")
	if len(parts) != 8 {
		t.Fatalf("expected 8 fields, got %d: %q", len(parts), got)
	}
}
