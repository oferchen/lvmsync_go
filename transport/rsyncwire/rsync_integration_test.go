//go:build integration && rsync

package rsyncwire

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gokrazy/rsync"
	"go.uber.org/zap"

	"lvmsync_go/common"
	"lvmsync_go/transport"
)

func TestNegotiateWithRsyncDaemon(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	dir := t.TempDir()
	modPath := filepath.Join(dir, "mod")
	if err := os.Mkdir(modPath, 0o755); err != nil {
		t.Fatalf("mkdir mod: %v", err)
	}
	conf := filepath.Join(dir, "rsyncd.conf")
	if err := os.WriteFile(conf, []byte("use chroot = false\n[test]\n  path = "+modPath+"\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	cmd := exec.CommandContext(ctx, "rsync", "--daemon", "--no-detach", "--port", strconv.Itoa(port), "--config", conf)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start rsync: %v", err)
	}
	t.Cleanup(func() {
		cmd.Process.Kill()
		cmd.Wait()
	})

	// Wait for daemon readiness.
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	var readyErr error
	for i := 0; i < 50; i++ {
		c, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			c.Close()
			readyErr = nil
			break
		}
		readyErr = err
		time.Sleep(100 * time.Millisecond)
	}
	if readyErr != nil {
		t.Fatalf("daemon not ready: %v", readyErr)
	}

	tr, err := New(transport.Config{Logger: zap.NewNop(), AllowInsecure: true})
	if err != nil {
		t.Fatalf("new transport: %v", err)
	}
	conn, err := tr.Dial(ctx, addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	if _, err := tr.Negotiate(ctx, conn, transport.Client, common.Handshake{}); err != nil {
		t.Fatalf("negotiate: %v", err)
	}
}

func TestNegotiateBadGreeting(t *testing.T) {
	c1, c2 := net.Pipe()
	go func() {
		c2.Write([]byte("bad\n"))
		c2.Close()
	}()
	tr, err := New(transport.Config{Logger: zap.NewNop(), AllowInsecure: true})
	if err != nil {
		t.Fatalf("new transport: %v", err)
	}
	if _, err := tr.Negotiate(context.Background(), c1, transport.Client, common.Handshake{}); err == nil {
		t.Fatalf("expected error")
	}
}

func TestServerHandshake(t *testing.T) {
	c1, c2 := net.Pipe()
	tr, err := New(transport.Config{Logger: zap.NewNop(), AllowInsecure: true})
	if err != nil {
		t.Fatalf("new transport: %v", err)
	}
	done := make(chan error)
	go func() {
		_, err := tr.Negotiate(context.Background(), c2, transport.Server, common.Handshake{})
		done <- err
	}()
	rd := bufio.NewReader(c1)
	line, err := rd.ReadString('\n')
	if err != nil {
		t.Fatalf("read greeting: %v", err)
	}
	if !strings.HasPrefix(line, "@RSYNCD:") {
		t.Fatalf("unexpected greeting %q", line)
	}
	fmt.Fprintf(c1, "@RSYNCD: %d\n\n", rsync.ProtocolVersion)
	resp, err := rd.ReadString('\n')
	if err != nil {
		t.Fatalf("read resp: %v", err)
	}
	if strings.TrimSpace(resp) != "@RSYNCD: EXIT" {
		t.Fatalf("unexpected resp %q", resp)
	}
	c1.Close()
	if err := <-done; err != nil {
		t.Fatalf("server negotiate: %v", err)
	}
}
