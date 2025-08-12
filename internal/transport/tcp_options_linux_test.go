//go:build linux

package transport

import (
	"net"
	"testing"
)

func TestSetNotSentLowAt(t *testing.T) {
	ln, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Skipf("listen failed: %v", err)
	}
	defer ln.Close()
	errCh := make(chan error, 1)
	go func() {
		c, err := net.Dial("tcp", ln.Addr().String())
		if err != nil {
			errCh <- err
			return
		}
		c.Close()
		errCh <- nil
	}()
	conn, err := ln.AcceptTCP()
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	defer conn.Close()
	if err := SetNotSentLowAt(conn, 1024); err != nil {
		t.Fatalf("SetNotSentLowAt: %v", err)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("dial: %v", err)
	}
}
