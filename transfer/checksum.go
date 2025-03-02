// transfer/checksum.go
package transfer

import (
	"crypto/md5"
	"crypto/sha256"
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

func GetChecksumStrategy(algo string) ChecksumStrategy {
	switch algo {
	case "md5":
		return &MD5Checksum{}
	default:
		return &SHA256Checksum{}
	}
}
