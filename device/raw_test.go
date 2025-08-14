package device

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenRawRejectsRegularFile(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "file")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}
	f.Close()
	if _, err := OpenRaw(f.Name(), true, "", ""); err == nil {
		t.Fatalf("expected error for regular file")
	}
}

func TestOpenRawRejectsCharDevice(t *testing.T) {
	if _, err := os.Stat("/dev/null"); err == nil {
		if _, err := OpenRaw("/dev/null", true, "", ""); err == nil {
			t.Fatalf("expected error for char device")
		}
	} else if os.IsNotExist(err) {
		t.Skip("/dev/null not found")
	}
}

func TestOpenRawRequiresOfflineOrFreeze(t *testing.T) {
	if _, err := OpenRaw("/dev/null", false, "", ""); err == nil {
		t.Fatalf("expected offline or freeze command error")
	}
}

func TestOpenRawFreezeCommandFailure(t *testing.T) {
	if _, err := OpenRaw("/dev/null", false, "false", "true"); err == nil {
		t.Fatalf("expected freeze command failure")
	}
}

func TestOpenRawRunsFreezeCommand(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "freeze")
	freezeCmd := fmt.Sprintf("touch %s", tmp)
	if _, err := OpenRaw("/dev/null", false, freezeCmd, "true"); err == nil {
		t.Fatalf("expected error for char device")
	}
	if _, err := os.Stat(tmp); err != nil {
		t.Fatalf("freeze command did not run")
	}
}

func TestCleanupRunsThawCommand(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "thaw")
	d := &RawDevice{fsThawCmd: fmt.Sprintf("touch %s", tmp), freezeIssued: true}
	if err := d.Cleanup(context.Background()); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if _, err := os.Stat(tmp); err != nil {
		t.Fatalf("thaw command did not run")
	}
}

func TestCleanupThawCommandFailure(t *testing.T) {
	d := &RawDevice{fsThawCmd: "false", freezeIssued: true}
	if err := d.Cleanup(context.Background()); err == nil {
		t.Fatalf("expected thaw command failure")
	}
}
