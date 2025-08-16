package rsynkserver

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/gokrazy/rsync"
	"go.uber.org/zap"

	"lvmsync_go/internal/digest"
	"lvmsync_go/internal/rsynkwire"
)

// Device represents a writable block device supporting random writes and sync.
//
// Implementations must write the provided data at the given offset and ensure
// persistence when Sync is called.
type Device interface {
	io.ReaderAt
	io.WriterAt
	Sync() error
	Size() int64
}

// Server applies incoming deltas to the Device.
//
// Frames are expected to be sent using rsynkwire.Stream and consist of a leading
// byte indicating the frame type:
//
//	'S' - signature frame used to reconstruct source signatures
//	'D' - delta frame containing an 8 byte big endian offset followed by data
//
// CRC32C integrity of each frame is verified by rsynkwire.Stream.
type Server struct {
	dev     Device
	alg     string
	expect  [32]byte
	logger  *zap.Logger
	sigHead *rsync.SumHead
}

// New constructs a Server using the provided Device, digest algorithm, expected
// manifest digest, and logger. A nil logger is replaced with zap.NewNop().
func New(dev Device, alg string, expect [32]byte, logger *zap.Logger) *Server {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Server{dev: dev, alg: alg, expect: expect, logger: logger}
}

// Handle consumes frames from the Stream until EOF, applying any delta frames to
// the Device. On graceful EOF the Device is fsynced.
func (s *Server) Handle(ctx context.Context, stream *rsynkwire.Stream) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		frame, err := stream.Recv()
		if err == io.EOF {
			if err := s.dev.Sync(); err != nil {
				return err
			}
			return s.verifyDigest()
		}
		if err != nil {
			return err
		}
		if len(frame) == 0 {
			continue
		}
		switch frame[0] {
		case 'S':
			head, err := parseSignatures(frame[1:])
			if err != nil {
				return err
			}
			s.sigHead = &head
			continue
		case 'D':
			if len(frame) < 9 {
				return fmt.Errorf("delta frame too short")
			}
			off := int64(binary.BigEndian.Uint64(frame[1:9]))
			data := frame[9:]
			size := s.dev.Size()
			dataLen := int64(len(data))
			if off < 0 || off > size-dataLen {
				s.logger.Warn("delta_out_of_bounds",
					zap.Int64("offset_bytes", off),
					zap.Int("data_size_bytes", len(data)),
					zap.Int64("device_size_bytes", size))
				return fmt.Errorf("delta out of bounds")
			}
			if _, err := s.dev.WriteAt(data, off); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unknown frame type %q", frame[0])
		}
	}
}

func parseSignatures(p []byte) (rsync.SumHead, error) {
	var head rsync.SumHead
	buf := bytes.NewReader(p)
	if err := binary.Read(buf, binary.LittleEndian, &head.ChecksumCount); err != nil {
		return head, err
	}
	if err := binary.Read(buf, binary.LittleEndian, &head.BlockLength); err != nil {
		return head, err
	}
	if err := binary.Read(buf, binary.LittleEndian, &head.ChecksumLength); err != nil {
		return head, err
	}
	if err := binary.Read(buf, binary.LittleEndian, &head.RemainderLength); err != nil {
		return head, err
	}
	head.Sums = make([]rsync.SumBuf, head.ChecksumCount)
	for i := int32(0); i < head.ChecksumCount; i++ {
		var short int32
		if err := binary.Read(buf, binary.LittleEndian, &short); err != nil {
			return head, err
		}
		strong := make([]byte, head.ChecksumLength)
		if _, err := io.ReadFull(buf, strong); err != nil {
			return head, err
		}
		blockLen := int64(head.BlockLength)
		if i == head.ChecksumCount-1 && head.RemainderLength != 0 {
			blockLen = int64(head.RemainderLength)
		}
		var strongArr [16]byte
		copy(strongArr[:], strong)
		head.Sums[i] = rsync.SumBuf{
			Offset: int64(i) * int64(head.BlockLength),
			Len:    blockLen,
			Index:  i,
			Sum1:   uint32(short),
			Sum2:   strongArr,
		}
	}
	return head, nil
}

func (s *Server) verifyDigest() error {
	r := io.NewSectionReader(s.dev, 0, s.dev.Size())
	got, err := digest.SumReader(r, s.alg)
	if err != nil {
		s.logger.Error("digest_compute_failed", zap.Error(err))
		return err
	}
	if got != s.expect {
		s.logger.Error("digest_mismatch",
			zap.String("algorithm", s.alg),
			zap.String("expected_digest", fmt.Sprintf("%x", s.expect)),
			zap.String("actual_digest", fmt.Sprintf("%x", got)))
		return fmt.Errorf("digest mismatch")
	}
	return nil
}
