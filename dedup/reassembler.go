package dedup

import "io"

// Reassemble reconstructs the original byte stream using the manifest and a
// reader that yields the unique chunks in the same order they were written
// by the replicator. Duplicate chunks are fetched from an internal cache
// keyed by their hash. The cache maps hashes to indices of the unique chunk
// list to avoid resending duplicate data.
func Reassemble(man Manifest, unique io.Reader, w io.Writer) error {
	cache := make(map[[32]byte]int)
	uniqueChunks := make([][]byte, 0)
	for _, e := range man.Chunks {
		idx, ok := cache[e.Hash]
		var data []byte
		if !ok {
			data = make([]byte, e.Length)
			if _, err := io.ReadFull(unique, data); err != nil {
				return err
			}
			uniqueChunks = append(uniqueChunks, data)
			cache[e.Hash] = len(uniqueChunks) - 1
		} else {
			data = uniqueChunks[idx]
		}
		if _, err := w.Write(data); err != nil {
			return err
		}
	}
	return nil
}
