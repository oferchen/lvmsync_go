package remote

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"io"

	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"
	"golang.org/x/sys/unix"
)

// PrivHelperClient manages communication with the privileged helper over an exec channel.
type PrivHelperClient struct {
	stdin   io.WriteCloser
	stdout  io.Reader
	session *ssh.Session
	logger  *zap.Logger
}

// StartPrivHelper starts the remote privileged helper using the provided SSH client.
// The helper reads (index, payload, hash) messages and performs pwrite operations.
// The provided context controls the lifetime of the startup; if it is canceled
// before the remote command begins executing, an error is returned.
func StartPrivHelper(ctx context.Context, client *ssh.Client, command string, logger *zap.Logger) (*PrivHelperClient, error) {
	if logger == nil {
		logger = zap.NewNop()
	}
	session, err := client.NewSession()
	if err != nil {
		return nil, err
	}
	stdin, err := session.StdinPipe()
	if err != nil {
		session.Close() //nolint:errcheck
		return nil, err
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		stdin.Close()   //nolint:errcheck
		session.Close() //nolint:errcheck
		return nil, err
	}
	errCh := make(chan error, 1)
	go func() { errCh <- session.Start(command) }()
	select {
	case <-ctx.Done():
		stdin.Close()   //nolint:errcheck
		session.Close() //nolint:errcheck
		return nil, ctx.Err()
	case err := <-errCh:
		if err != nil {
			stdin.Close()   //nolint:errcheck
			session.Close() //nolint:errcheck
			return nil, err
		}
	}
	return &PrivHelperClient{stdin: stdin, stdout: stdout, session: session, logger: logger}, nil
}

func (c *PrivHelperClient) send(offset uint64, payload []byte, hash [32]byte) error {
	var buf bytes.Buffer
	if err := binary.Write(&buf, binary.BigEndian, offset); err != nil {
		return err
	}
	if err := binary.Write(&buf, binary.BigEndian, uint32(len(payload))); err != nil {
		return err
	}
	buf.Write(hash[:])
	buf.Write(payload)
	_, err := c.stdin.Write(buf.Bytes())
	return err
}

// Send queues a write operation at the given offset with the provided payload.
// The payload's SHA-256 hash is calculated and sent to the helper for verification.
func (c *PrivHelperClient) Send(offset uint64, payload []byte) error {
	hash := sha256.Sum256(payload)
	return c.send(offset, payload, hash)
}

// RecvAck reads the next acknowledgment from the helper.
// It returns true for ACK and false for NACK.
func (c *PrivHelperClient) RecvAck() (bool, error) {
	var b [1]byte
	if _, err := io.ReadFull(c.stdout, b[:]); err != nil {
		return false, err
	}
	switch b[0] {
	case 'A':
		return true, nil
	case 'N':
		return false, nil
	default:
		return false, errors.New("invalid ack byte")
	}
}

// Close closes the helper session.
func (c *PrivHelperClient) Close() error {
	_ = c.stdin.Close()
	if err := c.session.Wait(); err != nil && !errors.Is(err, io.EOF) {
		_ = c.session.Close()
		return err
	}
	return c.session.Close()
}

// PrivilegedPwriteServer processes (index, payload, hash) messages from r and writes
// them to the file descriptor fd using pwrite. ACK ("A") is written to w on success
// and NACK ("N") on failure.
func PrivilegedPwriteServer(rw io.ReadWriter, fd int) error {
	header := make([]byte, 8+4+32) // offset uint64, length uint32, hash [32]byte
	for {
		if _, err := io.ReadFull(rw, header); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		offset := binary.BigEndian.Uint64(header[0:8])
		length := binary.BigEndian.Uint32(header[8:12])
		var expHash [32]byte
		copy(expHash[:], header[12:44])
		payload := make([]byte, length)
		if _, err := io.ReadFull(rw, payload); err != nil {
			return err
		}
		calc := sha256.Sum256(payload)
		if !bytes.Equal(expHash[:], calc[:]) {
			if _, werr := rw.Write([]byte{'N'}); werr != nil {
				return werr
			}
			continue
		}
		if _, err := unix.Pwrite(fd, payload, int64(offset)); err != nil {
			if _, werr := rw.Write([]byte{'N'}); werr != nil {
				return werr
			}
			continue
		}
		if _, err := rw.Write([]byte{'A'}); err != nil {
			return err
		}
	}
}
