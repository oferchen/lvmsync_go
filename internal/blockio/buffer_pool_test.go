package blockio

import (
	"os"
	"testing"
	"unsafe"
)

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
