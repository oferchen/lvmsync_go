// transfer/checksum.go
package transfer

import (
	"crypto/sha256"
	"strings"
	"sync"

	"github.com/zeebo/blake3"

	cpufeatures "lvmsync_go/internal/cpufeatures"
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

type BLAKE3Checksum struct{ size int }

func (b *BLAKE3Checksum) Compute(data []byte) []byte {
	if b.size > 32 {
		sum := blake3.Sum512(data)
		return sum[:]
	}
	sum := blake3.Sum256(data)
	return sum[:]
}

func (b *BLAKE3Checksum) Size() int { return b.size }

var (
	strategies map[string]ChecksumStrategy
	initOnce   sync.Once
)

var detectChecksumAlgorithm = func() string {
	if cpufeatures.HasAESNI() || cpufeatures.HasSIMD() {
		return "blake3"
	}
	return "sha256"
}

func initChecksumStrategies() {
	strategies = map[string]ChecksumStrategy{
		"sha256":     &SHA256Checksum{},
		"blake3":     &BLAKE3Checksum{size: 32},
		"blake3-256": &BLAKE3Checksum{size: 32},
		"blake3-512": &BLAKE3Checksum{size: 64},
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
