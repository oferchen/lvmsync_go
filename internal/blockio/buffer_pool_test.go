package blockio

import (
	"golang.org/x/sys/unix"
	"os"
	"testing"
	"unsafe"
)

func purgeAlignedBlockBufferPools() {
	bufferPools.Range(func(k, v any) bool {
		bufferPools.Delete(k)
		return true
	})
	mmappedBuffers.Range(func(k, v any) bool {
		if b, ok := v.([]byte); ok {
			unix.Munmap(b)
		}
		mmappedBuffers.Delete(k)
		return true
	})
}

func TestPutUnmapsWhenPoolMissing(t *testing.T) {
	size := os.Getpagesize()
	t.Cleanup(purgeAlignedBlockBufferPools)
	buf := getAlignedBlockBuffer(size)
	ptr := uintptr(unsafe.Pointer(&buf[0]))
	if _, ok := mmappedBuffers.Load(ptr); !ok {
		t.Fatalf("expected buffer to be mmapped")
	}
	bufferPools.Delete(size)
	putAlignedBlockBuffer(buf)
	if _, ok := mmappedBuffers.Load(ptr); ok {
		t.Fatalf("expected buffer to be unmapped when pool missing")
	}
}

func TestPurgeAlignedBlockBufferPools(t *testing.T) {
	size := os.Getpagesize()
	buf := getAlignedBlockBuffer(size)
	ptr := uintptr(unsafe.Pointer(&buf[0]))
	if _, ok := mmappedBuffers.Load(ptr); !ok {
		t.Fatalf("expected buffer to be mmapped")
	}
	putAlignedBlockBuffer(buf)
	purgeAlignedBlockBufferPools()
	if _, ok := mmappedBuffers.Load(ptr); ok {
		t.Fatalf("expected buffer to be unmapped after purge")
	}
	if _, ok := bufferPools.Load(size); ok {
		t.Fatalf("expected buffer pool to be purged")
	}
}
