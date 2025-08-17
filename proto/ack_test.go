package proto

import "testing"

func TestAckDefaults(t *testing.T) {
	var a Ack
	if a.GetSessionId() != "" || a.GetMessage() != "" || a.GetOk() {
		t.Fatalf("unexpected defaults: %+v", a)
	}
}
