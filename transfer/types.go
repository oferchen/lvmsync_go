// transfer/types.go
package transfer

import wal "github.com/oferchen/lvmsync_go/internal/wal"

type Range = wal.Range

type BlockTask struct {
	Index int
	R     Range
}

type BlockResult struct {
	Index   int
	Offset  uint64
	Size    uint32
	Data    []byte
	ChunkID [32]byte
	Err     error
}
