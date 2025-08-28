package transfer

import (
	"errors"
	"os"
	"syscall"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/oferchen/lvmsync_go/internal/config"
)

func TestSetupPipeZeroCopyEnabled(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{ZeroCopy: true}
	core, logs := observer.New(zap.WarnLevel)
	logger := zap.New(core)

	pipeFds, cleanup, err := setupPipe(cfg, logger)
	if err != nil {
		t.Fatalf("setupPipe returned error: %v", err)
	}
	if pipeFds[0] < 0 || pipeFds[1] < 0 {
		t.Fatalf("invalid pipe fds: %v", pipeFds)
	}

	// ensure cleanup closes descriptors
	cleanup()
	if err := syscall.Close(pipeFds[0]); !errors.Is(err, syscall.EBADF) {
		t.Fatalf("expected fd0 closed, got %v", err)
	}
	if err := syscall.Close(pipeFds[1]); !errors.Is(err, syscall.EBADF) {
		t.Fatalf("expected fd1 closed, got %v", err)
	}
	if logs.Len() != 0 {
		t.Fatalf("expected no warnings, got %d", logs.Len())
	}
}

func TestSetupPipePipeFailure(t *testing.T) {
	cfg := &config.Config{ZeroCopy: true}
	core, logs := observer.New(zap.WarnLevel)
	logger := zap.New(core)

	var orig syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &orig); err != nil {
		t.Fatalf("getrlimit: %v", err)
	}
	// restrict the soft limit to the current number of open descriptors
	fds, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	lim := orig
	lim.Cur = uint64(len(fds))
	if err := syscall.Setrlimit(syscall.RLIMIT_NOFILE, &lim); err != nil {
		t.Fatalf("setrlimit: %v", err)
	}
	defer syscall.Setrlimit(syscall.RLIMIT_NOFILE, &orig)

	_, cleanup, err := setupPipe(cfg, logger)
	if err == nil {
		t.Skip("pipe creation succeeded despite RLIMIT_NOFILE limit")
	}
	// cleanup should be safe and produce no warnings
	cleanup()
	if logs.Len() != 0 {
		t.Fatalf("expected no warnings, got %d", logs.Len())
	}
}

func TestSetupPipeZeroCopyDisabled(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{ZeroCopy: false}
	core, logs := observer.New(zap.WarnLevel)
	logger := zap.New(core)

	pipeFds, cleanup, err := setupPipe(cfg, logger)
	if err != nil {
		t.Fatalf("setupPipe returned error: %v", err)
	}
	if pipeFds[0] != -1 || pipeFds[1] != -1 {
		t.Fatalf("expected pipe fds to be -1, got %v", pipeFds)
	}
	cleanup()
	if logs.Len() != 0 {
		t.Fatalf("expected no warnings, got %d", logs.Len())
	}
}
