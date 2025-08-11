// Package compress provides pluggable compression codecs with an auto mode that
// selects between LZ4 and Zstd based on input size.
package compress

// Codec compresses and decompresses byte slices.
type Codec interface {
	Compress(in []byte) (out []byte, used bool, err error)
	Decompress(in []byte) ([]byte, error)
}
