//go:build root

package escalate

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func TestEscalateAndDrop_Root(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root")
	}
	cmd := exec.Command(os.Args[0], "-test.run", "TestHelperEscalateAndDrop")
	cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("helper: %v\n%s", err, out.String())
	}
	logs := out.String()
	if !strings.Contains(logs, "\"msg\":\"escalation_success\"") {
		t.Fatalf("missing escalation_success log: %s", logs)
	}
	if !strings.Contains(logs, "\"msg\":\"deescalation_success\"") {
		t.Fatalf("missing deescalation_success log: %s", logs)
	}
	if !strings.Contains(logs, "\"new_euid\":1") {
		t.Fatalf("expected new_euid=1 in logs: %s", logs)
	}
}

func TestHelperEscalateAndDrop(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	logger := zap.New(zapcore.NewCore(
		zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
		os.Stdout,
		zapcore.InfoLevel,
	))
	defer logger.Sync()

	reexeced, err := EnsureRootOrReexec(Options{})
	if err != nil || reexeced {
		logger.Error("escalation_failed", zap.Bool("reexeced", reexeced), zap.Error(err))
		os.Exit(1)
	}
	logger.Info("escalation_success", zap.Int("euid", os.Geteuid()))

	os.Setenv("SUDO_UID", "1")
	os.Setenv("SUDO_GID", "1")
	if err := DropToInvokerIfSudo(Options{}); err != nil {
		logger.Error("deescalation_failed", zap.Error(err))
		os.Exit(2)
	}
	logger.Info("deescalation_success", zap.Int("new_euid", os.Geteuid()))
	_ = logger.Sync()
	os.Exit(0)
}
