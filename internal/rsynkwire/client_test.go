package rsynkwire

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"testing"

	"github.com/gokrazy/rsync"
)

const maxFrame = 1 << 20

func TestClientSendSignatures(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	client := NewClient(NewStream(c1, maxFrame))
	srv := NewStream(c2, maxFrame)

	data := []byte("testdata")
	headExpect := sumSizesSqroot(int64(len(data)))
	errCh := make(chan error)
	go func() {
		frame, err := srv.Recv()
		if err != nil {
			errCh <- err
			return
		}
		if frame[0] != 'S' {
			errCh <- fmt.Errorf("unexpected frame type %q", frame[0])
			return
		}
		buf := bytes.NewReader(frame[1:])
		var head rsync.SumHead
		if err := binary.Read(buf, binary.LittleEndian, &head.ChecksumCount); err != nil {
			errCh <- err
			return
		}
		if err := binary.Read(buf, binary.LittleEndian, &head.BlockLength); err != nil {
			errCh <- err
			return
		}
		if err := binary.Read(buf, binary.LittleEndian, &head.ChecksumLength); err != nil {
			errCh <- err
			return
		}
		if err := binary.Read(buf, binary.LittleEndian, &head.RemainderLength); err != nil {
			errCh <- err
			return
		}
		if head.ChecksumCount != headExpect.ChecksumCount ||
			head.BlockLength != headExpect.BlockLength ||
			head.ChecksumLength != headExpect.ChecksumLength ||
			head.RemainderLength != headExpect.RemainderLength {
			errCh <- fmt.Errorf("unexpected head %+v want %+v", head, headExpect)
			return
		}
		var short int32
		if err := binary.Read(buf, binary.LittleEndian, &short); err != nil {
			errCh <- err
			return
		}
		strong := make([]byte, head.ChecksumLength)
		if _, err := io.ReadFull(buf, strong); err != nil {
			errCh <- err
			return
		}
		if uint32(short) != checksum1(data) {
			errCh <- fmt.Errorf("checksum1 mismatch")
			return
		}
		if !bytes.Equal(strong, checksum2(data)[:head.ChecksumLength]) {
			errCh <- fmt.Errorf("checksum2 mismatch")
			return
		}
		errCh <- nil
	}()

	if _, err := client.SendSignatures(bytes.NewReader(data)); err != nil {
		t.Fatalf("SendSignatures: %v", err)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("verify: %v", err)
	}
}

func TestStreamBadCRC(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	s := NewStream(c2, maxFrame)
	payload := []byte("bad")
	var hdr [8]byte
	binary.BigEndian.PutUint32(hdr[0:4], uint32(len(payload)))
	binary.BigEndian.PutUint32(hdr[4:8], 0)
	if _, err := c1.Write(hdr[:]); err != nil {
		t.Fatalf("write hdr: %v", err)
	}
	if _, err := c1.Write(payload); err != nil {
		t.Fatalf("write payload: %v", err)
	}
	if _, err := s.Recv(); err == nil {
		t.Fatalf("expected CRC error")
	}
}
