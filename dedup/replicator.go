package dedup

import (
	"encoding/binary"
	"io"

	"go.uber.org/zap"

	"lvmsync_go/hash"
)

// Replicator wires together the chunker, BLAKE3 hasher and Bloom filter into a
// forward-only streaming pipeline. XXH3 hashes provide deduplication hints
// while BLAKE3 digests are recorded in the manifest. Unique chunks are written
// to the provided writer while all chunks are recorded into the manifest.
// ChunkSource supplies content-defined chunks from an input reader.
type ChunkSource interface {
	NextChunk(r io.Reader) (Chunk, error)
}

type Replicator struct {
	Chunker  ChunkSource
	hasher   *hash.Blake3Hasher
	Bloom    *Bloom
	Writer   io.Writer
	Manifest *Manifest
	Logger   *zap.Logger
}

// NewReplicator creates a new replicator with the supplied components.
func NewReplicator(ch ChunkSource, h *hash.Blake3Hasher, b *Bloom, w io.Writer, logger *zap.Logger) *Replicator {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Replicator{Chunker: ch, hasher: h, Bloom: b, Writer: w, Manifest: &Manifest{}, Logger: logger}
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
		r.hasher.Reset()
		if _, err = r.hasher.Write(chunk.Data); err != nil {
			return Manifest{}, err
		}
		digest := r.hasher.Sum256()
		xx := hash.SumXXH3(chunk.Data)
		var buf [8]byte
		binary.LittleEndian.PutUint64(buf[:], xx)
		if !r.Bloom.TestAndAdd(buf[:]) {
			if _, err = r.Writer.Write(chunk.Data); err != nil {
				return Manifest{}, err
			}
		}
		r.Manifest.Append(digest, offset, chunk.Length)
		offset += int64(chunk.Length)
		if err == io.EOF {
			break
		}
	}
	r.Manifest.AuditLog(r.Logger)
	return *r.Manifest, nil
}
