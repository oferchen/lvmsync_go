package dedup

import "io"

// Reassemble reconstructs the original byte stream using the manifest and a
// reader that yields the unique chunks in the same order they were written
// by the replicator. Duplicate chunks are fetched from an internal cache
// keyed by their hash.
func Reassemble(man Manifest, unique io.Reader, w io.Writer) error {
	cache := make(map[[32]byte][]byte)
	for _, e := range man.Chunks {
		data, ok := cache[e.Hash]
		if !ok {
			data = make([]byte, e.Length)
			if _, err := io.ReadFull(unique, data); err != nil {
				return err
			}
			cache[e.Hash] = data
		}
		if _, err := w.Write(data); err != nil {
			return err
		}
	}
	return nil
}
