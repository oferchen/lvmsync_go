package rsynkwire

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"runtime"
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

func TestClientSendSignaturesLargeInput(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	client := NewClient(NewStream(c1, maxFrame))
	srv := NewStream(c2, maxFrame)

	// Consume the frame so the client can write without blocking.
	errCh := make(chan error, 1)
	go func() {
		_, err := srv.Recv()
		errCh <- err
	}()

	// Create a large zeroed buffer and measure memory before and after
	// sending signatures to ensure we don't retain the entire input.
	const size = 32 << 20 // 32MiB
	data := make([]byte, size)
	r := bytes.NewReader(data)

	runtime.GC()
	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)
	if _, err := client.SendSignatures(r); err != nil {
		t.Fatalf("SendSignatures: %v", err)
	}
	runtime.GC()
	var m2 runtime.MemStats
	runtime.ReadMemStats(&m2)
	if err := <-errCh; err != nil {
		t.Fatalf("recv: %v", err)
	}

	// Expect substantially less additional memory than the input size.
	if diff := int64(m2.Alloc) - int64(m1.Alloc); diff > 5<<20 {
		t.Fatalf("unexpected memory use: %d", diff)
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
	errCh := make(chan error, 1)
	go func() {
		if _, err := c1.Write(hdr[:]); err != nil {
			errCh <- fmt.Errorf("write hdr: %w", err)
			return
		}
		if _, err := c1.Write(payload); err != nil {
			errCh <- fmt.Errorf("write payload: %w", err)
			return
		}
		errCh <- c1.Close()
	}()
	if _, err := s.Recv(); err == nil {
		t.Fatalf("expected CRC error")
	}
	if err := <-errCh; err != nil {
		t.Fatalf("write: %v", err)
	}
}
