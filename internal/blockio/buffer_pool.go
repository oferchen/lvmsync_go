package blockio

import (
	"sync"

	"golang.org/x/sys/unix"
)

var bufferPools sync.Map

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
		}
	}
}
