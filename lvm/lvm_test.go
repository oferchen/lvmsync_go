package lvm

import "testing"

func TestGetSnapshotDevicePath(t *testing.T) {
	got := GetSnapshotDevicePath("snap", "vg0")
	if got != "/dev/vg0/snap" {
		t.Fatalf("unexpected path %s", got)
	}
}

func TestSetGetEscalationCommand(t *testing.T) {
	SetEscalationCommand("sudo")
	if GetEscalationCommand() != "sudo" {
		t.Fatalf("expected sudo, got %s", GetEscalationCommand())
	}
}
