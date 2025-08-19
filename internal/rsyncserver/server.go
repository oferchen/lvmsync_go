package rsyncserver

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"math"

	"github.com/gokrazy/rsync"
	"go.uber.org/zap"

	"lvmsync_go/internal/digest"
	"lvmsync_go/internal/rsyncwire"
	"lvmsync_go/internal/signaturecache"
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
// Frames are expected to be sent using rsyncwire.Stream and consist of a leading
// byte indicating the frame type:
//
//	'S' - signature frame used to reconstruct source signatures
//	'D' - delta frame containing an 8 byte big endian offset followed by data
//
// CRC32C integrity of each frame is verified by rsyncwire.Stream.
type Server struct {
	dev     Device
	alg     string
	expect  [32]byte
	logger  *zap.Logger
	sigHead *rsync.SumHead
	cache   *signaturecache.Cache
	vg      string
	lv      string
}

// New constructs a Server using the provided Device and logger.
// The digest algorithm and expected sum are supplied via a later digest frame.
func New(dev Device, logger *zap.Logger, cache *signaturecache.Cache, vg, lv string) *Server {
	return &Server{dev: dev, logger: logger, cache: cache, vg: vg, lv: lv}
}

// Handle consumes frames from the Stream until EOF, applying any delta frames to
// the Device. On graceful EOF the Device is fsynced.
func (s *Server) Handle(ctx context.Context, stream *rsyncwire.Stream) error {
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
			head, err := parseSignatures(frame[1:], s.dev.Size())
			if err != nil {
				return err
			}
			s.sigHead = &head
			continue
		case 'D':
			if len(frame) < 9 {
				return fmt.Errorf("delta frame too short")
			}
			u := binary.BigEndian.Uint64(frame[1:9])
			if u > math.MaxInt64 {
				s.logger.Error("delta_out_of_bounds",
					zap.Uint64("offset_bytes", u),
					zap.Int("delta_size_bytes", len(frame)-9),
					zap.Int64("device_size_bytes", s.dev.Size()))
				return fmt.Errorf("delta offset overflows int64")
			}
			off := int64(u)
			data := frame[9:]
			size := s.dev.Size()
			dataLen := int64(len(data))
			end := off + dataLen
			if off < 0 || end < 0 || end > size {
				s.logger.Error("delta_out_of_bounds",
					zap.Int64("offset_bytes", off),
					zap.Int64("delta_size_bytes", dataLen),
					zap.Int64("end_offset_bytes", end),
					zap.Int64("device_size_bytes", size))
				return fmt.Errorf("delta out of bounds")
			}
			n, err := s.dev.WriteAt(data, off)
			if err != nil {
				return err
			}
			if n != len(data) {
				return fmt.Errorf("short write: wrote %d of %d bytes", n, len(data))
			}
		case 'G':
			if len(frame) < 2+32 {
				return fmt.Errorf("digest frame too short")
			}
			algLen := int(frame[1])
			if len(frame) != 2+algLen+32 {
				return fmt.Errorf("digest frame length mismatch")
			}
			s.alg = string(frame[2 : 2+algLen])
			copy(s.expect[:], frame[2+algLen:])
			if s.cache != nil && s.cache.Check(s.vg, s.lv, s.dev.Size(), s.expect[:]) {
				return nil
			}
			continue
		default:
			return fmt.Errorf("unknown frame type %q", frame[0])
		}
	}
}

func parseSignatures(p []byte, devSize int64) (rsync.SumHead, error) {
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
	if head.BlockLength <= 0 {
		return head, fmt.Errorf("invalid block length")
	}
	max := devSize / int64(head.BlockLength)
	if devSize%int64(head.BlockLength) != 0 || head.RemainderLength != 0 {
		max++
	}
	if int64(head.ChecksumCount) > max {
		return head, fmt.Errorf("checksum count %d exceeds limit %d", head.ChecksumCount, max)
	}
	remaining := len(p) - 16
	need := int(head.ChecksumCount) * (4 + int(head.ChecksumLength))
	if remaining < need {
		return head, fmt.Errorf("signature frame too short")
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
	if s.alg == "" {
		return fmt.Errorf("missing digest frame")
	}
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
	if s.cache != nil {
		if err := s.cache.Put(s.vg, s.lv, s.dev.Size(), got[:]); err != nil {
			s.logger.Warn("cache_put_failed", zap.Error(err))
		}
	}
	return nil
}
