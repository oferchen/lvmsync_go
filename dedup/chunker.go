package dedup

import (
	"io"
	"math"

	"lvmsync_go/common"
)

// Chunk represents a block of data detected by the chunker.
type Chunk struct {
	Offset int64
	Length int
	Data   []byte
}

// Chunker implements a FastCDC style content defined chunker with
// entropy aware sizing. The implementation is streaming and does not
// require seeking on the underlying reader.
type Chunker struct {
	Min int
	Avg int
	Max int

	// internal state
	maskNormal uint64
	maskHigh   uint64
	maskLow    uint64
	window     [64]byte // used for entropy estimation
}

// NewChunker returns a new chunker configured with the provided
// minimum, average, and maximum chunk sizes. All sizes are in bytes. The mask values are
// derived from the average size.
func NewChunker(minSize, avgSize, maxSize int) *Chunker {
	c := &Chunker{Min: minSize, Avg: avgSize, Max: maxSize}
	// derive masks for different entropy levels. The mask controls the
	// probability of finding a boundary. Higher mask -> larger chunks.
	bits := uint64(math.Log2(float64(avgSize)))
	if bits < 1 {
		bits = 1
	}
	c.maskNormal = (1 << bits) - 1
	if bits > 2 {
		c.maskHigh = (1 << (bits - 2)) - 1 // small chunks
	} else {
		c.maskHigh = 0
	}
	c.maskLow = (1 << (bits + 2)) - 1 // large chunks
	return c
}

// NewChunkerFromHandshake constructs a Chunker using CDC parameters negotiated
// via a protocol handshake.
func NewChunkerFromHandshake(h common.Handshake) *Chunker {
	return NewChunker(h.CDCMin, h.CDCAvg, h.CDCMax)
}

// NextChunk reads from r and returns the next content defined chunk. It
// returns io.EOF when no more data is available. The returned chunk's
// Data slice is owned by the caller and will be reused by the chunker on
// subsequent calls; copy it if persistence is required.
//
//nolint:revive // algorithmic complexity required for chunking
func (c *Chunker) NextChunk(r io.Reader) (Chunk, error) {
	buf := make([]byte, c.Max)
	n, err := io.ReadFull(r, buf[:c.Min])
	if err != nil {
		if err == io.ErrUnexpectedEOF || err == io.EOF {
			if n == 0 {
				return Chunk{}, io.EOF
			}
			return Chunk{Length: n, Data: buf[:n]}, io.EOF
		}
		return Chunk{}, err
	}

	size := c.Min
	var h uint64
	var b [1]byte

	// initialize entropy window
	copy(c.window[:], buf[size-64:size])
	counts := [256]int{}
	for _, b := range c.window {
		counts[b]++
	}

	for size < c.Max {
		_, err = r.Read(b[:])
		if err != nil {
			if err == io.EOF {
				break
			}
			return Chunk{}, err
		}
		buf[size] = b[0]
		size++
		// update rolling hash
		h = (h << 1) + gear[b[0]]
		// update entropy window and determine mask
		e := c.updateEntropy(b[0], &counts)
		mask := c.selectMask(e)

		if size >= c.Min && h&mask == 0 {
			break
		}
	}
	return Chunk{Length: size, Data: buf[:size]}, nil
}

// updateEntropy updates the rolling entropy window with the new byte and
// returns the current entropy.
func (c *Chunker) updateEntropy(b byte, counts *[256]int) float64 {
	out := c.window[0]
	copy(c.window[:63], c.window[1:])
	c.window[63] = b
	counts[out]--
	counts[b]++
	return entropy(counts[:])
}

// selectMask chooses the appropriate mask based on the entropy value.
func (c *Chunker) selectMask(e float64) uint64 {
	switch {
	case e < 4.0:
		return c.maskLow
	case e > 7.0:
		return c.maskHigh
	default:
		return c.maskNormal
	}
}

// FastCDC chunks the entirety of r using the FastCDC algorithm with the
// provided size targets. It returns all detected chunks.
func FastCDC(r io.Reader, min, avg, max int) ([]Chunk, error) {
	ch := NewChunker(min, avg, max)
	var out []Chunk
	var offset int64
	for {
		c, err := ch.NextChunk(r)
		if err == io.EOF && c.Length == 0 {
			break
		}
		if err != nil && err != io.EOF {
			return nil, err
		}
		c.Offset = offset
		offset += int64(c.Length)
		out = append(out, c)
		if err == io.EOF {
			break
		}
	}
	return out, nil
}

// gear table taken from FastCDC reference implementation.
var gear = [256]uint64{
	0x9ae16a3b2f90404f, 0x4f1bbf83b5dc07d3, 0x5c6bfb31e933b7f1, 0x81f69c5e0d6cc818,
	0x8ec3f371df537f7d, 0x9cd55d72b9a5f8c4, 0xeee0b0f8bf26d835, 0xa7b5d8e7cd3f2c09,
	0xc4d3b2e0f1a59786, 0xd1258b8a3f1c9b0d, 0xe91a1c1b2c3d4e5f, 0xfedcba9876543210,
	0x0123456789abcdef, 0x02468aceeca86420, 0x13579bdf2468ace0, 0x0f1e2d3c4b5a6978,
	0x89abcdef01234567, 0xa1b2c3d4e5f60718, 0xbad0c0ffee0d1e2f, 0xcafebabefeedface,
	0xd4c3b2a1908f7e6d, 0xe3f2c1b0a9876543, 0xf1e0d2c3b4a59687, 0x1234567890abcdef,
	0x23456789abcdef01, 0x3456789abcdef012, 0x456789abcdef0123, 0x56789abcdef01234,
	0x6789abcdef012345, 0x789abcdef0123456, 0x89abcdef01234567, 0x9abcdef012345678,
	0xabcdef0123456789, 0xbcdef0123456789a, 0xcdef0123456789ab, 0xdef0123456789abc,
	0xef0123456789abcd, 0xf0123456789abcde, 0x0123456789abcdef, 0x89abcdef01234567,
	0xfedcba9876543210, 0x10fedcba98765432, 0x3210fedcba987654, 0x543210fedcba9876,
	0x76543210fedcba98, 0x9876543210fedcba, 0xba9876543210fedc, 0xdcba9876543210fe,
	0xfedcba9876543210, 0x0f1e2d3c4b5a6978, 0x13579bdf2468ace0, 0x02468aceeca86420,
	0xabcdef0123456789, 0x9abcdef012345678, 0x89abcdef01234567, 0x789abcdef0123456,
	0x6789abcdef012345, 0x56789abcdef01234, 0x456789abcdef0123, 0x3456789abcdef012,
	0x23456789abcdef01, 0x1234567890abcdef, 0xf1e0d2c3b4a59687, 0xe3f2c1b0a9876543,
	0xd4c3b2a1908f7e6d, 0xcafebabefeedface, 0xbad0c0ffee0d1e2f, 0xa1b2c3d4e5f60718,
	0x89abcdef01234567, 0x0f1e2d3c4b5a6978, 0x13579bdf2468ace0, 0x02468aceeca86420,
	0x0123456789abcdef, 0xfedcba9876543210, 0xe91a1c1b2c3d4e5f, 0xd1258b8a3f1c9b0d,
	0xc4d3b2e0f1a59786, 0xa7b5d8e7cd3f2c09, 0xeee0b0f8bf26d835, 0x9cd55d72b9a5f8c4,
	0x8ec3f371df537f7d, 0x81f69c5e0d6cc818, 0x5c6bfb31e933b7f1, 0x4f1bbf83b5dc07d3,
	0x9ae16a3b2f90404f, 0x3210fedcba987654, 0x23456789abcdef01, 0x0123456789abcdef,
	0xfedcba9876543210, 0xcafebabefeedface, 0x89abcdef01234567, 0x456789abcdef0123,
	0x89abcdef01234567, 0xf0123456789abcde, 0x9abcdef012345678, 0x6789abcdef012345,
	0x456789abcdef0123, 0x23456789abcdef01, 0x0123456789abcdef, 0xef0123456789abcd,
	0xcdef0123456789ab, 0xabcdef0123456789, 0x89abcdef01234567, 0x6789abcdef012345,
	0x456789abcdef0123, 0x23456789abcdef01, 0x0123456789abcdef, 0xfedcba9876543210,
	0x76543210fedcba98, 0x543210fedcba9876, 0x3210fedcba987654, 0x10fedcba98765432,
	0xfedcba9876543210, 0xdcba9876543210fe, 0xba9876543210fedc, 0x9876543210fedcba,
	0x76543210fedcba98, 0x543210fedcba9876, 0x3210fedcba987654, 0x10fedcba98765432,
	0xfedcba9876543210, 0xdcba9876543210fe, 0xba9876543210fedc, 0x9876543210fedcba,
	0x9ae16a3b2f90404f, 0x4f1bbf83b5dc07d3, 0x5c6bfb31e933b7f1, 0x81f69c5e0d6cc818,
	0x8ec3f371df537f7d, 0x9cd55d72b9a5f8c4, 0xeee0b0f8bf26d835, 0xa7b5d8e7cd3f2c09,
	0xc4d3b2e0f1a59786, 0xd1258b8a3f1c9b0d, 0xe91a1c1b2c3d4e5f, 0xfedcba9876543210,
	0x0123456789abcdef, 0x02468aceeca86420, 0x13579bdf2468ace0, 0x0f1e2d3c4b5a6978,
	0x89abcdef01234567, 0xa1b2c3d4e5f60718, 0xbad0c0ffee0d1e2f, 0xcafebabefeedface,
	0xd4c3b2a1908f7e6d, 0xe3f2c1b0a9876543, 0xf1e0d2c3b4a59687, 0x1234567890abcdef,
	0x23456789abcdef01, 0x3456789abcdef012, 0x456789abcdef0123, 0x56789abcdef01234,
	0x6789abcdef012345, 0x789abcdef0123456, 0x89abcdef01234567, 0x9abcdef012345678,
	0xabcdef0123456789, 0xbcdef0123456789a, 0xcdef0123456789ab, 0xdef0123456789abc,
	0xef0123456789abcd, 0xf0123456789abcde, 0x0123456789abcdef, 0x89abcdef01234567,
	0xfedcba9876543210, 0x10fedcba98765432, 0x3210fedcba987654, 0x543210fedcba9876,
	0x76543210fedcba98, 0x9876543210fedcba, 0xba9876543210fedc, 0xdcba9876543210fe,
	0xfedcba9876543210, 0x0f1e2d3c4b5a6978, 0x13579bdf2468ace0, 0x02468aceeca86420,
	0xabcdef0123456789, 0x9abcdef012345678, 0x89abcdef01234567, 0x789abcdef0123456,
	0x6789abcdef012345, 0x56789abcdef01234, 0x456789abcdef0123, 0x3456789abcdef012,
	0x23456789abcdef01, 0x1234567890abcdef, 0xf1e0d2c3b4a59687, 0xe3f2c1b0a9876543,
	0xd4c3b2a1908f7e6d, 0xcafebabefeedface, 0xbad0c0ffee0d1e2f, 0xa1b2c3d4e5f60718,
	0x89abcdef01234567, 0x0f1e2d3c4b5a6978, 0x13579bdf2468ace0, 0x02468aceeca86420,
	0x0123456789abcdef, 0xfedcba9876543210, 0xe91a1c1b2c3d4e5f, 0xd1258b8a3f1c9b0d,
	0xc4d3b2e0f1a59786, 0xa7b5d8e7cd3f2c09, 0xeee0b0f8bf26d835, 0x9cd55d72b9a5f8c4,
	0x8ec3f371df537f7d, 0x81f69c5e0d6cc818, 0x5c6bfb31e933b7f1, 0x4f1bbf83b5dc07d3,
	0x9ae16a3b2f90404f, 0x3210fedcba987654, 0x23456789abcdef01, 0x0123456789abcdef,
	0xfedcba9876543210, 0xcafebabefeedface, 0x89abcdef01234567, 0x456789abcdef0123,
	0x89abcdef01234567, 0xf0123456789abcde, 0x9abcdef012345678, 0x6789abcdef012345,
	0x456789abcdef0123, 0x23456789abcdef01, 0x0123456789abcdef, 0xef0123456789abcd,
	0xcdef0123456789ab, 0xabcdef0123456789, 0x89abcdef01234567, 0x6789abcdef012345,
	0x456789abcdef0123, 0x23456789abcdef01, 0x0123456789abcdef, 0xfedcba9876543210,
	0x76543210fedcba98, 0x543210fedcba9876, 0x3210fedcba987654, 0x10fedcba98765432,
	0xfedcba9876543210, 0xdcba9876543210fe, 0xba9876543210fedc, 0x9876543210fedcba,
	0x76543210fedcba98, 0x543210fedcba9876, 0x3210fedcba987654, 0x10fedcba98765432,
	0xfedcba9876543210, 0xdcba9876543210fe, 0xba9876543210fedc, 0x9876543210fedcba,
}

// entropy returns an approximate Shannon entropy for the 64 byte window.
func entropy(counts []int) float64 {
	total := 64.0
	var e float64
	for _, c := range counts {
		if c == 0 {
			continue
		}
		p := float64(c) / total
		e -= p * math.Log2(p)
	}
	return e
}
