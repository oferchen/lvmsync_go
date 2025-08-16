package rsynkwire

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"math"

	"github.com/gokrazy/rsync"
	"github.com/mmcloughlin/md4"
)

var crcTable = crc32.MakeTable(crc32.Castagnoli)

// Stream wraps an io.ReadWriter and transmits length-prefixed frames
// with a prepended CRC32C. Each Write becomes a single frame and Recv
// yields fully validated frames to callers.
type Stream struct {
	rw  io.ReadWriter
	max uint32
}

// NewStream wraps the provided io.ReadWriter with CRC32C framing.
// max specifies the maximum accepted frame size.
func NewStream(rw io.ReadWriter, max uint32) *Stream { return &Stream{rw: rw, max: max} }

// Send writes a single frame to the underlying stream, prefixing the
// payload with its length and CRC32C checksum.
func (s *Stream) Send(p []byte) error {
	var hdr [8]byte
	binary.BigEndian.PutUint32(hdr[0:4], uint32(len(p)))
	binary.BigEndian.PutUint32(hdr[4:8], crc32.Checksum(p, crcTable))
	if _, err := s.rw.Write(hdr[:]); err != nil {
		return err
	}
	if len(p) > 0 {
		if _, err := s.rw.Write(p); err != nil {
			return err
		}
	}
	return nil
}

// Recv reads the next frame from the stream, verifying the CRC32C value.
// It returns io.EOF when the underlying stream is exhausted.
func (s *Stream) Recv() ([]byte, error) {
	var hdr [8]byte
	if _, err := io.ReadFull(s.rw, hdr[:]); err != nil {
		return nil, err
	}
	n := binary.BigEndian.Uint32(hdr[0:4])
	if n > s.max {
		return nil, fmt.Errorf("frame %d exceeds max %d", n, s.max)
	}
	expected := binary.BigEndian.Uint32(hdr[4:8])
	buf := make([]byte, n)
	if _, err := io.ReadFull(s.rw, buf); err != nil {
		return nil, err
	}
	if crc32.Checksum(buf, crcTable) != expected {
		return nil, fmt.Errorf("crc32c mismatch")
	}
	return buf, nil
}

// Client transmits rsync signatures and deltas over a Stream.
type Client struct {
	stream *Stream
}

// NewClient constructs a Client using the provided Stream.
func NewClient(stream *Stream) *Client { return &Client{stream: stream} }

// SendSignatures reads all data from r, computes rsync signatures and sends
// them as a single frame prefixed with 'S'. It returns the generated SumHead.
func (c *Client) SendSignatures(r io.Reader) (rsync.SumHead, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return rsync.SumHead{}, err
	}
	head := sumSizesSqroot(int64(len(data)))
	head.Sums = make([]rsync.SumBuf, int(head.ChecksumCount))
	var offset int64
	for i := int32(0); i < head.ChecksumCount; i++ {
		blockLen := int64(head.BlockLength)
		if i == head.ChecksumCount-1 && head.RemainderLength != 0 {
			blockLen = int64(head.RemainderLength)
		}
		chunk := data[offset : offset+blockLen]
		sum1 := checksum1(chunk)
		sum2 := checksum2(chunk)
		var s2arr [16]byte
		copy(s2arr[:], sum2[:])
		head.Sums[i] = rsync.SumBuf{
			Offset: offset,
			Len:    blockLen,
			Index:  i,
			Sum1:   sum1,
			Sum2:   s2arr,
		}
		offset += blockLen
	}

	var buf bytes.Buffer
	binary.Write(&buf, binary.LittleEndian, head.ChecksumCount)
	binary.Write(&buf, binary.LittleEndian, head.BlockLength)
	binary.Write(&buf, binary.LittleEndian, head.ChecksumLength)
	binary.Write(&buf, binary.LittleEndian, head.RemainderLength)
	for _, sb := range head.Sums {
		binary.Write(&buf, binary.LittleEndian, int32(sb.Sum1))
		buf.Write(sb.Sum2[:head.ChecksumLength])
	}
	payload := append([]byte{'S'}, buf.Bytes()...)
	if err := c.stream.Send(payload); err != nil {
		return rsync.SumHead{}, err
	}
	return head, nil
}

// SendDelta sends a delta frame indicating data to be written at the provided
// offset. The frame is prefixed with 'D'.
func (c *Client) SendDelta(offset int64, data []byte) error {
	var buf bytes.Buffer
	buf.WriteByte('D')
	binary.Write(&buf, binary.BigEndian, uint64(offset))
	buf.Write(data)
	return c.stream.Send(buf.Bytes())
}

// sumSizesSqroot mirrors rsync's block size and count calculation.
func sumSizesSqroot(contentLen int64) rsync.SumHead {
	const minBlock = 700
	blockLength := int32(math.Max(math.Sqrt(float64(contentLen)), minBlock))
	const checksumLength = 16
	return rsync.SumHead{
		ChecksumCount:   int32((contentLen + int64(blockLength) - 1) / int64(blockLength)),
		RemainderLength: int32(contentLen % int64(blockLength)),
		BlockLength:     blockLength,
		ChecksumLength:  checksumLength,
	}
}

func signExtend(b byte) uint32 {
	val := uint32(b)
	return uint32(int32(val<<24) >> 24)
}

func checksum1(buf []byte) uint32 {
	bufLen := len(buf)
	var s1, s2 uint32
	var i int
	if bufLen > 4 {
		for i = 0; i < bufLen-4; i += 4 {
			s2 += 4*(s1+signExtend(buf[i])) +
				3*signExtend(buf[i+1]) +
				2*signExtend(buf[i+2]) +
				signExtend(buf[i+3])
			s1 += signExtend(buf[i]) +
				signExtend(buf[i+1]) +
				signExtend(buf[i+2]) +
				signExtend(buf[i+3])
		}
	}
	for ; i < bufLen; i++ {
		s1 += signExtend(buf[i])
		s2 += s1
	}
	return (s1 & 0xffff) + (s2 << 16)
}

func checksum2(buf []byte) []byte {
	h := md4.New()
	h.Write(buf)
	binary.Write(h, binary.LittleEndian, int32(0))
	return h.Sum(nil)
}
