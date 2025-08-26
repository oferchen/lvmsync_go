package blockio

import (
	"sync"
	"unsafe"

	"golang.org/x/sys/unix"
)

var (
	bufferPools    sync.Map
	mmappedBuffers sync.Map // key uintptr -> []byte
)

func getAlignedBlockBuffer(size int) []byte {
	if p, ok := bufferPools.Load(size); ok {
		if pool, ok := p.(*sync.Pool); ok {
			if bufAny := pool.Get(); bufAny != nil {
				if bp, ok := bufAny.(*[]byte); ok {
					return *bp
				}
			}
		}
	}
	pool := &sync.Pool{New: func() any {
		b, err := unix.Mmap(-1, 0, size, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_PRIVATE|unix.MAP_ANON)
		if err != nil {
			buf := make([]byte, size)
			return &buf
		}
		if len(b) > 0 {
			ptr := uintptr(unsafe.Pointer(&b[0]))
			mmappedBuffers.Store(ptr, b)
		}
		return &b
	}}
	actual, _ := bufferPools.LoadOrStore(size, pool)
	bufAny := actual.(*sync.Pool).Get()
	if bp, ok := bufAny.(*[]byte); ok {
		return *bp
	}
	if b, ok := bufAny.([]byte); ok {
		return b
	}
	buf := make([]byte, size)
	return buf
}

func putAlignedBlockBuffer(buf []byte) {
	if p, ok := bufferPools.Load(len(buf)); ok {
		if pool, ok := p.(*sync.Pool); ok {
			pool.Put(&buf)
			return
		}
	}
	if len(buf) > 0 {
		ptr := uintptr(unsafe.Pointer(&buf[0]))
		if bAny, ok := mmappedBuffers.LoadAndDelete(ptr); ok {
			unix.Munmap(bAny.([]byte))
		}
	}
}
