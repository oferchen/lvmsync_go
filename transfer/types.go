// transfer/types.go
package transfer

type Range struct {
	Start uint64
	End   uint64
}

type BlockTask struct {
	Index int
	R     Range
}

type BlockResult struct {
	Index  int
	Offset uint64
	Size   uint32
	Data   []byte
	Err    error
}
