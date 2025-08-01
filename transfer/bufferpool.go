package transfer

import "sync"

var bufferPools sync.Map

func getPool(size int) *sync.Pool {
	if p, ok := bufferPools.Load(size); ok {
		return p.(*sync.Pool)
	}
	p := &sync.Pool{New: newBufferFunc(size)}
	actual, _ := bufferPools.LoadOrStore(size, p)
	return actual.(*sync.Pool)
}

func newBufferFunc(size int) func() interface{} {
	return func() interface{} {
		return make([]byte, size)
	}
}

func getBlockBuffer(size int) []byte {
	return getPool(size).Get().([]byte)
}

func putBlockBuffer(buf []byte) {
	getPool(len(buf)).Put(buf)
}
