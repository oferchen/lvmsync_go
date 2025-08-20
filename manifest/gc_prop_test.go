package manifest

import (
	"bytes"
	"math/rand"
	"path/filepath"
	"reflect"
	"testing"
	"testing/quick"

	"github.com/zeebo/blake3"
	"github.com/zeebo/xxh3"
	"golang.org/x/sys/unix"
)

const (
	propBlockSize = 4096
	propChunks    = 32
)

type gcOp struct {
	Delete bool
	Index  uint16
	Data   []byte
}

type gcSeq []gcOp

func (gcSeq) Generate(r *rand.Rand, size int) reflect.Value {
	n := r.Intn(50)
	seq := make(gcSeq, n)
	for i := 0; i < n; i++ {
		op := gcOp{
			Delete: r.Intn(2) == 0,
			Index:  uint16(r.Intn(propChunks)),
		}
		if !op.Delete {
			l := r.Intn(64) + 1
			op.Data = make([]byte, l)
			_, _ = r.Read(op.Data)
		}
		seq[i] = op
	}
	return reflect.ValueOf(seq)
}

type liveEntry struct {
	length uint32
	digest [32]byte
}

func TestGCProp(t *testing.T) {
	cfg := &quick.Config{MaxCount: 100}

	if err := quick.Check(func(ops gcSeq) bool {
		dir := t.TempDir()
		path := filepath.Join(dir, "gc.man")
		size := uint64(propBlockSize * propChunks)
		idx, err := Create(path, "dev", size, 0, 0, 0, propBlockSize, 0, 0, 0, 0)
		if err != nil {
			t.Logf("Create: %v", err)
			return false
		}

		live := make(map[uint64]liveEntry)
		for _, op := range ops {
			offset := uint64(op.Index) * propBlockSize
			if op.Delete {
				if err := idx.Set(offset, 0, 0, 0, [32]byte{}); err != nil {
					t.Logf("delete set: %v", err)
					return false
				}
				delete(live, offset)
				continue
			}
			dig := blake3.Sum256(op.Data)
			xxh := xxh3.Hash(op.Data)
			if err := idx.Set(offset, uint32(len(op.Data)), 0, xxh, dig); err != nil {
				t.Logf("set: %v", err)
				return false
			}
			live[offset] = liveEntry{uint32(len(op.Data)), dig}
		}
		if err := unix.Msync(idx.data, unix.MS_SYNC); err != nil {
			t.Logf("msync: %v", err)
			return false
		}
		if err := idx.f.Sync(); err != nil {
			t.Logf("fsync: %v", err)
			return false
		}
		if err := idx.Close(); err != nil {
			t.Logf("close: %v", err)
			return false
		}

		if err := GC(path); err != nil {
			t.Logf("gc: %v", err)
			return false
		}
		idx2, err := Open(path)
		if err != nil {
			t.Logf("open after gc: %v", err)
			return false
		}
		defer idx2.Close()

		for i := uint64(0); i < idx2.ChunkCount(); i++ {
			off, length, _, _, dig, err := idx2.Entry(i)
			if err != nil {
				t.Logf("entry %d: %v", i, err)
				return false
			}
			if length == 0 {
				if _, ok := live[off]; ok {
					t.Logf("live entry missing at offset %d", off)
					return false
				}
				continue
			}
			exp, ok := live[off]
			if !ok {
				t.Logf("unexpected entry at offset %d", off)
				return false
			}
			if length != exp.length || !bytes.Equal(dig[:], exp.digest[:]) {
				t.Logf("mismatch at offset %d", off)
				return false
			}
			delete(live, off)
		}
		if len(live) != 0 {
			t.Logf("missing %d entries", len(live))
			return false
		}
		return true
	}, cfg); err != nil {
		t.Fatalf("quick.Check failed: %v", err)
	}
}
