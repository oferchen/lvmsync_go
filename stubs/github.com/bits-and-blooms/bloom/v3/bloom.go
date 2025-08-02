package bloom

import (
	"bytes"
	"encoding/binary"
	"io"
)

type BloomFilter struct {
	items map[string]struct{}
}

func NewWithEstimates(n uint, p float64) *BloomFilter {
	return &BloomFilter{items: make(map[string]struct{})}
}

func (b *BloomFilter) Add(data []byte) {
	if b.items == nil {
		b.items = make(map[string]struct{})
	}
	b.items[string(data)] = struct{}{}
}

func (b *BloomFilter) Test(data []byte) bool {
	if b.items == nil {
		return false
	}
	_, ok := b.items[string(data)]
	return ok
}

func (b *BloomFilter) WriteTo(w io.Writer) (int64, error) {
	var buf bytes.Buffer
	for k := range b.items {
		binary.Write(&buf, binary.LittleEndian, uint32(len(k)))
		buf.WriteString(k)
	}
	n, err := w.Write(buf.Bytes())
	return int64(n), err
}

func (b *BloomFilter) ReadFrom(r io.Reader) (int64, error) {
	if b.items == nil {
		b.items = make(map[string]struct{})
	}
	var total int64
	for {
		var l uint32
		if err := binary.Read(r, binary.LittleEndian, &l); err != nil {
			if err == io.EOF {
				break
			}
			return total, err
		}
		buf := make([]byte, l)
		if _, err := io.ReadFull(r, buf); err != nil {
			return total, err
		}
		b.items[string(buf)] = struct{}{}
		total += int64(4 + l)
	}
	return total, nil
}
