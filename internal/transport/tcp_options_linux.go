//go:build linux

package transport

import (
	"net"

	"golang.org/x/sys/unix"
)

// SetNotSentLowAt applies TCP_NOTSENT_LOWAT to the provided TCP connection when
// value > 0. A value of 0 leaves the option unchanged.
func SetNotSentLowAt(conn *net.TCPConn, value int) error {
	if value <= 0 {
		return nil
	}
	fd, err := conn.SyscallConn()
	if err != nil {
		return err
	}
	var setErr error
	if err := fd.Control(func(fd uintptr) {
		setErr = unix.SetsockoptInt(int(fd), unix.IPPROTO_TCP, unix.TCP_NOTSENT_LOWAT, value)
	}); err != nil {
		return err
	}
	return setErr
}
