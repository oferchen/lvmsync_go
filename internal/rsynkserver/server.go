package rsynkserver

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

// New constructs a Server using the provided Device and logger. The digest
// algorithm and expected sum are supplied via a later digest frame. A nil
// logger is replaced with zap.NewNop().
func New(dev Device, logger *zap.Logger) *Server {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Server{dev: dev, logger: logger}
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
	return nil
}
