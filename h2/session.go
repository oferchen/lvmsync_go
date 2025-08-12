package h2

import (
	"errors"
	"sync"
)

// Session holds state for a resumable transfer. Each bit in Bitmap tracks
// whether the corresponding chunk has been uploaded.
type Session struct {
	id        string
	size      int
	chunkSize int
	data      []byte
	bitmap    []byte
	mu        sync.Mutex
}

// newSession initializes a Session for the given id, total size and chunk size.
func newSession(id string, size, chunkSize int) *Session {
	chunkCount := (size + chunkSize - 1) / chunkSize
	return &Session{
		id:        id,
		size:      size,
		chunkSize: chunkSize,
		data:      make([]byte, size),
		bitmap:    make([]byte, (chunkCount+7)/8),
	}
}

// Upload writes b into the session at the provided range.
func (s *Session) Upload(start, end int, b []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if start < 0 || end >= s.size || end < start {
		return errors.New("invalid range")
	}
	if len(b) != end-start+1 {
		return errors.New("length mismatch")
	}
	copy(s.data[start:end+1], b)
	// mark bits
	first := start / s.chunkSize
	last := end / s.chunkSize
	for i := first; i <= last; i++ {
		byteIdx := i / 8
		bit := uint(i % 8)
		s.bitmap[byteIdx] |= 1 << bit
	}
	return nil
}

// Download returns the data for the requested range.
func (s *Session) Download(start, end int) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if start < 0 || end >= s.size || end < start {
		return nil, errors.New("invalid range")
	}
	out := make([]byte, end-start+1)
	copy(out, s.data[start:end+1])
	return out, nil
}

// Bitmap returns a copy of the current bitmap.
func (s *Session) Bitmap() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := make([]byte, len(s.bitmap))
	copy(cp, s.bitmap)
	return cp
}
