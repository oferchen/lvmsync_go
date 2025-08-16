package digest

import (
	"crypto/sha256"
	"fmt"
	"hash"
	"io"
	"os"

	"github.com/zeebo/blake3"
)

// newHasher returns a hash.Hash for the given algorithm.
func newHasher(alg string) (hash.Hash, error) {
	switch alg {
	case SHA256:
		return sha256.New(), nil
	case BLAKE3:
		return blake3.New(), nil
	default:
		return nil, fmt.Errorf("unsupported digest algorithm: %s", alg)
	}
}

// SumReader computes the digest of data from r using the specified algorithm.
func SumReader(r io.Reader, alg string) ([32]byte, error) {
	h, err := newHasher(alg)
	if err != nil {
		return [32]byte{}, err
	}
	if _, err := io.Copy(h, r); err != nil {
		return [32]byte{}, err
	}
	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out, nil
}

// SumFile computes the digest of the file at path using the specified algorithm.
func SumFile(path, alg string) ([32]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return [32]byte{}, err
	}
	defer f.Close()
	return SumReader(f, alg)
}

// sampleSize is the number of bytes read from the start and end of a file when
// performing sampled verification.
const sampleSize int64 = 1 << 20 // 1MiB

// sampledSumFile computes a digest of the first and last sampleSize bytes of
// the file at path using the specified algorithm. If the file is smaller than
// 2*sampleSize, the entire file is hashed.
func sampledSumFile(path, alg string) ([32]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return [32]byte{}, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return [32]byte{}, err
	}
	h, err := newHasher(alg)
	if err != nil {
		return [32]byte{}, err
	}
	buf := make([]byte, sampleSize)
	n, err := f.Read(buf)
	if err != nil && err != io.EOF {
		return [32]byte{}, err
	}
	h.Write(buf[:n])
	if info.Size() > sampleSize*2 {
		if _, err := f.Seek(-sampleSize, io.SeekEnd); err != nil {
			return [32]byte{}, err
		}
		n, err = f.Read(buf)
		if err != nil && err != io.EOF {
			return [32]byte{}, err
		}
		h.Write(buf[:n])
	}
	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out, nil
}

// VerifyFiles compares digests of src and dst using the specified algorithm.
// If sampled is true, only the first and last sampleSize bytes are hashed.
// It returns whether the digests match along with the individual digests.
func VerifyFiles(src, dst, alg string, sampled bool) (bool, [32]byte, [32]byte, error) {
	var s1, s2 [32]byte
	var err error
	if sampled {
		s1, err = sampledSumFile(src, alg)
		if err != nil {
			return false, s1, s2, err
		}
		s2, err = sampledSumFile(dst, alg)
		if err != nil {
			return false, s1, s2, err
		}
	} else {
		s1, err = SumFile(src, alg)
		if err != nil {
			return false, s1, s2, err
		}
		s2, err = SumFile(dst, alg)
		if err != nil {
			return false, s1, s2, err
		}
	}
	return s1 == s2, s1, s2, nil
}
