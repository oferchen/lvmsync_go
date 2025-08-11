// Package compress includes an auto codec that picks LZ4 for smaller inputs and
// Zstd for larger ones, returning the original data when compression is not
// beneficial.
package compress

// Auto selects a codec based on input size.
type Auto struct {
	lz4  Codec
	zstd Codec
}

// NewAuto returns an auto-selecting codec.
func NewAuto() Codec { return &Auto{lz4: NewLZ4(), zstd: NewZstd()} }

// Compress chooses LZ4 for inputs <256KiB otherwise Zstd.
func (a *Auto) Compress(in []byte) ([]byte, bool, error) {
	if len(in) < 256<<10 {
		return a.lz4.Compress(in)
	}
	return a.zstd.Compress(in)
}

// Decompress tries Zstd first then falls back to LZ4.
func (a *Auto) Decompress(in []byte) ([]byte, error) {
	if out, err := a.zstd.Decompress(in); err == nil {
		return out, nil
	}
	return a.lz4.Decompress(in)
}
