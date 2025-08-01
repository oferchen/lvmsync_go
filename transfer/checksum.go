// transfer/checksum.go
package transfer

import (
	"crypto/md5"
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

type MD5Checksum struct{}

func (m *MD5Checksum) Compute(data []byte) []byte {
	sum := md5.Sum(data)
	return sum[:]
}

var (
	sha256Instance ChecksumStrategy = &SHA256Checksum{}
	md5Instance    ChecksumStrategy = &MD5Checksum{}
	initOnce       sync.Once
)

func initChecksumInstances() {
	sha256Instance = &SHA256Checksum{}
	md5Instance = &MD5Checksum{}
}

func GetChecksumStrategy(algo string) ChecksumStrategy {
	initOnce.Do(initChecksumInstances)

	switch algo {
	case "md5":
		return md5Instance
	default:
		return sha256Instance
	}
}
