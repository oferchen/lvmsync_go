// Package compress includes an auto codec that selects a compression algorithm
// based on CPU features, falling back to LZ4 or Zstd.
package compress

import "lvmsync_go/internal/compressiondetect"

// Auto chooses a codec based on detected CPU characteristics.
type Auto struct {
	lz4  Codec
	zstd Codec
	use  string
}

// NewAuto returns an auto-selecting codec.
func NewAuto() Codec {
	algo := compressiondetect.DetectOptimalCompression()
	return &Auto{lz4: NewLZ4(), zstd: NewZstd(), use: algo}
}

// Compress uses the preferred codec.
func (a *Auto) Compress(in []byte) ([]byte, bool, error) {
	if a.use == "zstd" {
		return a.zstd.Compress(in)
	}
	return a.lz4.Compress(in)
}

// Decompress tries the preferred codec then falls back to the other.
func (a *Auto) Decompress(in []byte) ([]byte, error) {
	if a.use == "zstd" {
		if out, err := a.zstd.Decompress(in); err == nil {
			return out, nil
		}
		return a.lz4.Decompress(in)
	}
	if out, err := a.lz4.Decompress(in); err == nil {
		return out, nil
	}
	return a.zstd.Decompress(in)
}
