package remote

import (
	"fmt"
	"net"
	"os"
	"testing"

	"golang.org/x/crypto/ssh/knownhosts"
)

func createKnownHostsFile(t *testing.T, server *mockSSHServer) string {
	t.Helper()
	host, portStr, err := net.SplitHostPort(server.addr)
	if err != nil {
		t.Fatalf("SplitHostPort: %v", err)
	}
	line := knownhosts.Line([]string{fmt.Sprintf("[%s]:%s", host, portStr)}, server.publicKey)
	f, err := os.CreateTemp(t.TempDir(), "known_hosts")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	if _, err := f.WriteString(line + "\n"); err != nil {
		t.Fatalf("WriteString: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	return f.Name()
}

func createEmptyKnownHosts(t *testing.T) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "known_hosts")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	return f.Name()
}
