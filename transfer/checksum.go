// transfer/checksum.go
package transfer

import (
	"crypto/sha256"
	"strings"
	"sync"

	"github.com/zeebo/blake3"

	digest "lvmsync_go/internal/digest"
)

// ChecksumStrategy defines an interface for computing checksums with a
// predictable output size.
type ChecksumStrategy interface {
	Compute(data []byte) []byte
	Size() int
}

type SHA256Checksum struct{}

func (s *SHA256Checksum) Compute(data []byte) []byte {
	sum := sha256.Sum256(data)
	return sum[:]
}

func (s *SHA256Checksum) Size() int { return sha256.Size }

type BLAKE3Checksum struct{}

func (b *BLAKE3Checksum) Compute(data []byte) []byte {
	sum := blake3.Sum256(data)
	return sum[:]
}

func (b *BLAKE3Checksum) Size() int { return 32 }

var (
	strategies map[string]ChecksumStrategy
	initOnce   sync.Once
)

var detectChecksumAlgorithm = digest.Select

func initChecksumStrategies() {
	strategies = map[string]ChecksumStrategy{
		"sha256":     &SHA256Checksum{},
		"blake3":     &BLAKE3Checksum{},
		"blake3-256": &BLAKE3Checksum{},
	}
}

// GetChecksumStrategy returns a checksum strategy for the requested algorithm.
// An unknown algorithm defaults to SHA-256.
func GetChecksumStrategy(algo string) ChecksumStrategy {
	initOnce.Do(initChecksumStrategies)
	a := strings.ToLower(algo)
	if a == "" || a == StrategyAuto {
		a = detectChecksumAlgorithm()
	}
	if s, ok := strategies[a]; ok {
		return s
	}
	return strategies["sha256"]
}
