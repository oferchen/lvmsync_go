package device

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"
)

func TestOpenRawRejectsRegularFile(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "file")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	f.Close()
	if _, err := OpenRaw(context.Background(), f.Name(), true, "", "", zap.NewNop()); err == nil {
		t.Fatalf("expected error for regular file")
	}
}

func TestOpenRawRejectsCharDevice(t *testing.T) {
	if _, err := os.Stat("/dev/null"); err == nil {
		if _, err := OpenRaw(context.Background(), "/dev/null", true, "", "", zap.NewNop()); err == nil {
			t.Fatalf("expected error for char device")
		}
	} else if os.IsNotExist(err) {
		t.Skip("/dev/null not found")
	}
}

func TestOpenRawRequiresOfflineOrFreeze(t *testing.T) {
	if _, err := OpenRaw(context.Background(), "/dev/null", false, "", "", zap.NewNop()); err == nil {
		t.Fatalf("expected offline or freeze command error")
	}
}

func TestOpenRawFreezeCommandFailure(t *testing.T) {
	if _, err := OpenRaw(context.Background(), "/dev/null", false, "false", "true", zap.NewNop()); err == nil {
		t.Fatalf("expected freeze command failure")
	}
}

func TestOpenRawThawsOnFailure(t *testing.T) {
	freezeTmp := filepath.Join(t.TempDir(), "freeze")
	thawTmp := filepath.Join(t.TempDir(), "thaw")
	freezeCmd := fmt.Sprintf("touch %s", freezeTmp)
	thawCmd := fmt.Sprintf("touch %s", thawTmp)
	if _, err := OpenRaw(context.Background(), "/dev/null", false, freezeCmd, thawCmd, zap.NewNop()); err == nil {
		t.Fatalf("expected error for char device")
	}
	if _, err := os.Stat(freezeTmp); err != nil {
		t.Fatalf("freeze command did not run")
	}
	if _, err := os.Stat(thawTmp); err != nil {
		t.Fatalf("thaw command did not run")
	}
}

func TestCleanupRunsThawCommand(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "thaw")
	d := &RawDevice{fsThawCmd: fmt.Sprintf("touch %s", tmp), freezeIssued: true, logger: zap.NewNop()}
	if err := d.Cleanup(context.Background()); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if _, err := os.Stat(tmp); err != nil {
		t.Fatalf("thaw command did not run")
	}
}

func TestCleanupThawCommandFailure(t *testing.T) {
	d := &RawDevice{fsThawCmd: "false", freezeIssued: true, logger: zap.NewNop()}
	if err := d.Cleanup(context.Background()); err == nil {
		t.Fatalf("expected thaw command failure")
	}
}
