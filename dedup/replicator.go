package dedup

import (
	"io"

	"go.uber.org/zap"
)

// Replicator wires together the chunker, hasher and Bloom filter into a
// forward-only streaming pipeline. Unique chunks are written to the
// provided writer while all chunks are recorded into the manifest.
type Replicator struct {
	Chunker  *Chunker
	Hasher   *Hasher
	Bloom    *Bloom
	Writer   io.Writer
	Manifest *Manifest
}

// NewReplicator creates a new replicator with the supplied components.
func NewReplicator(ch *Chunker, h *Hasher, b *Bloom, w io.Writer) *Replicator {
	return &Replicator{Chunker: ch, Hasher: h, Bloom: b, Writer: w, Manifest: &Manifest{}}
}

// Process consumes data from r, chunking, hashing and deduplicating it in
// a streaming fashion. The manifest describing all chunks is returned.
func (r *Replicator) Process(src io.Reader) (Manifest, error) {
	var offset int64
	for {
		chunk, err := r.Chunker.NextChunk(src)
		if err == io.EOF && chunk.Length == 0 {
			break
		}
		r.Hasher.Reset()
		if _, err = r.Hasher.Write(chunk.Data); err != nil {
			return Manifest{}, err
		}
		hash := r.Hasher.Sum256()
		if !r.Bloom.TestAndAdd(hash[:]) {
			if _, err = r.Writer.Write(chunk.Data); err != nil {
				return Manifest{}, err
			}
		}
		r.Manifest.Append(hash, offset, chunk.Length)
		offset += int64(chunk.Length)
		if err == io.EOF {
			break
		}
	}
	r.Manifest.AuditLog(zap.L())
	return *r.Manifest, nil
}
