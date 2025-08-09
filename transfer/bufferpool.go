package transfer

import "sync"

var bufferPools sync.Map

func getPool(size int) *sync.Pool {
	if p, ok := bufferPools.Load(size); ok {
		if pool, ok := p.(*sync.Pool); ok {
			return pool
		}
	}
	p := &sync.Pool{New: func() any {
		buf := make([]byte, size)
		return &buf
	}}
	actual, _ := bufferPools.LoadOrStore(size, p)
	pool, ok := actual.(*sync.Pool)
	if !ok {
		return p
	}
	return pool
}

func getBlockBuffer(size int) []byte {
	pool := getPool(size)
	bufAny := pool.Get()
	bufPtr, ok := bufAny.(*[]byte)
	if !ok {
		b := make([]byte, size)
		return b
	}
	return *bufPtr
}

func putBlockBuffer(buf []byte) {
	getPool(len(buf)).Put(&buf)
}
