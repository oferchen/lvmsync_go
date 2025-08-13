package remote

import (
	"testing"

	remotetest "lvmsync_go/remote/testutil"
)

func TestSetupHostKeyCallbackInsecure(t *testing.T) {
	cb, err := setupHostKeyCallback(false, "")
	if err != nil {
		t.Fatalf("setupHostKeyCallback returned error: %v", err)
	}
	if err := cb("example.com", nil, nil); err != nil {
		t.Fatalf("insecure callback returned error: %v", err)
	}
}

func TestSetupHostKeyCallbackVerify(t *testing.T) {
	kh := remotetest.CreateEmptyKnownHosts(t)
	if _, err := setupHostKeyCallback(true, kh); err != nil {
		t.Fatalf("setupHostKeyCallback returned error: %v", err)
	}
}
