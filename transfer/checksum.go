// transfer/checksum.go
package transfer

import (
	"crypto/sha256"
	"sync"
)

type ChecksumStrategy interface {
	Compute(data []byte) []byte
}

type SHA256Checksum struct{}

func (s *SHA256Checksum) Compute(data []byte) []byte {
	sum := sha256.Sum256(data)
	return sum[:]
}

var (
	sha256Instance ChecksumStrategy = &SHA256Checksum{}
	initOnce       sync.Once
)

func initChecksumStrategies() {
	sha256Instance = &SHA256Checksum{}
}

func GetChecksumStrategy(algo string) ChecksumStrategy {
	initOnce.Do(initChecksumStrategies)
	// Only SHA-256 is supported; default to it for any request.
	return sha256Instance
}
