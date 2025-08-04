package lvm

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetSnapshotDevicePath(t *testing.T) {
	got := GetSnapshotDevicePath("snap", "vg0")
	if got != "/dev/vg0/snap" {
		t.Fatalf("unexpected path %s", got)
	}
}

func TestSetGetEscalationCommand(t *testing.T) {
	t.Cleanup(func() { SetEscalationCommand("") })
	SetEscalationCommand("sudo")
	if GetEscalationCommand() != "sudo" {
		t.Fatalf("expected sudo, got %s", GetEscalationCommand())
	}
}

func TestParseSnapshotSize(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "vol")
	if err := os.WriteFile(tmpFile, make([]byte, 1024*1024), 0644); err != nil {
		t.Fatalf("failed to create temp volume: %v", err)
	}

	tests := []struct {
		name    string
		input   string
		want    uint64
		wantErr bool
	}{
		{"percent", "50%", 512 * 1024, false},
		{"absolute", "1M", 1000000, false},
		{"invalidPercent", "150%", 0, true},
		{"invalidValue", "abc", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseSnapshotSize(tt.input, tmpFile)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseSnapshotSize error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && got != tt.want {
				t.Fatalf("got %d, want %d", got, tt.want)
			}
		})
	}

	Cleanup()
}

func TestGetVolumeSize(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "vol")
	if err := os.WriteFile(tmpFile, make([]byte, 2*1024*1024), 0644); err != nil {
		t.Fatalf("failed to create temp volume: %v", err)
	}

	size, err := GetVolumeSize(tmpFile)
	if err != nil {
		t.Fatalf("GetVolumeSize failed: %v", err)
	}
	if size != 2*1024*1024 {
		t.Fatalf("size = %d, want %d", size, 2*1024*1024)
	}

	Cleanup()
}

func TestGetVolumeAttributes(t *testing.T) {
	tmpDir := t.TempDir()
	SetSysBlockPath(tmpDir)
	defer SetSysBlockPath("/sys/block")

	devDir := filepath.Join(tmpDir, "testdev")
	if err := os.Mkdir(devDir, 0755); err != nil {
		t.Fatalf("failed to create device dir: %v", err)
	}

	attrs := map[string]string{
		"dev":       "8:1",
		"size":      "2048",
		"ro":        "0",
		"removable": "1",
	}

	for k, v := range attrs {
		if err := os.WriteFile(filepath.Join(devDir, k), []byte(v), 0644); err != nil {
			t.Fatalf("failed to write attribute %s: %v", k, err)
		}
	}

	got, err := GetVolumeAttributes("/dev/testdev")
	if err != nil {
		t.Fatalf("GetVolumeAttributes failed: %v", err)
	}

	for k, v := range attrs {
		if got[k] != v {
			t.Fatalf("attribute %s = %q, want %q", k, got[k], v)
		}
	}
}
