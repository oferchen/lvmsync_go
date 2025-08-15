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
	winPos     int      // current position in the entropy window

	// reusable buffer to avoid per-chunk allocations
	buf []byte
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
	if cap(c.buf) < c.Max {
		c.buf = make([]byte, c.Max)
	}
	buf := c.buf[:c.Max]
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
	c.winPos = 0
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
	out := c.window[c.winPos]
	c.window[c.winPos] = b
	c.winPos = (c.winPos + 1) % len(c.window)
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

// gear table of 256 random 64-bit values as specified by FastCDC.
var gear = [256]uint64{
	0x629f6fbed82c07cd, 0xe3e70682c2094cac, 0x0a5d2f346baa9455, 0xf728b4fa42485e3a,
	0x7c65c1e582e2e662, 0xeb1167b367a9c378, 0xd4713d60c8a70639, 0xf7c1bd874da5e709,
	0x5ba91faf7a024204, 0xe443df789558867f, 0x37ebdcd9e87a1613, 0x23a7711a81332876,
	0x23c6612f48268673, 0x1846d424c17c6279, 0xcca5a5a19e4d6e3c, 0xfcbd04c340212ef7,
	0x88561712e8e5216a, 0xb4862b21fb97d435, 0x9a164106cf6a659e, 0x259f4329e6f4590b,
	0x19488dec4f65d4d9, 0x12e0c8b2bad640fb, 0xd9b8a714e61a441c, 0x5487ce1eaf19922a,
	0x8f4ff31e78de5857, 0x5a92118719c78df4, 0x50f244556f25e2a2, 0xa3f2c9bf9c6316b9,
	0x3458a748e9bb17bc, 0x8d723104f77383c1, 0x71545a137a1d5006, 0x85776e9add84f39e,
	0x0ff18e0242af9fc3, 0xeb2083e6ce164dba, 0xea7e9d498c778ea6, 0x17e0aa3c03983ca8,
	0xd71037d1b83e90ec, 0xb5d32b1666194cb1, 0xc8f8e3d0d3290a4c, 0xa0116be5ab0c1681,
	0x9ca5499d004ae545, 0xd3fbf47a7e5b1e7f, 0x55485822de1b372a, 0xbaf3897a3e70f16a,
	0xb421eaeb534097ca, 0x101fbcccded733e8, 0xeac1c14f30e9c5cc, 0x38c1962e9148624f,
	0xcda8056c3d15eef7, 0x247a8333f7b0b7d2, 0x8b0163c1cd9d2b7d, 0x1759edc372ae2244,
	0xfe43c49e149818d1, 0xe005b86051ef1922, 0xff7b118e820865d6, 0x7d41e602eece328b,
	0x4d2b9deb1beb3711, 0x4a84eb038d1fd9b7, 0x1ff39849b4e1357d, 0x552f233a8c25166a,
	0xec188efbd080e66e, 0x3405095c8a5006c1, 0xcca74147f6be1f72, 0x8c1745a79a6a5f92,
	0x49a3e80e966e1277, 0x1775336d71eacd05, 0xcc45782198a6416d, 0x5129fb7c6288e1a5,
	0x3dfabc08935ddd72, 0x2f1205544a5308cc, 0xd24bace4307bf326, 0x0870e15c2fcd81b5,
	0xfb3675b89cdeb3e6, 0x42930b33a81ad477, 0x11af923d79fdef7c, 0xadc0da7a16febaa0,
	0x215663abc1f254b8, 0x2648ee38e07405eb, 0x09e469e6ec62b2c8, 0x148b2758d7ab7928,
	0xb306d1a8e5eeac76, 0xd450fe4aec4f217b, 0xaef9c00b8a64c1b9, 0xd67e55fd642bfa42,
	0x864a7a50b48d73f1, 0x85940927468ff53d, 0x3c49d76fcfc6e625, 0x37176e84d977e993,
	0xadf20806e5214606, 0xd344749096fd35d0, 0x6b5f5241f323ca74, 0x467437419466e472,
	0x7e1ea9c573581a81, 0xa425799aa905d750, 0xb341facdff0ac0f1, 0xfb82860deabca8d0,
	0x5b7c709acb175a5a, 0x5306f3f515166570, 0x1d878f9f9cdf5a86, 0x964a870c7c879b74,
	0x55d44936a1515607, 0x30bcab0ed8570102, 0x0426465e3e37952d, 0x4562be7fbb42e0b2,
	0xb490b6081dfc8352, 0x5f3f563838701a14, 0x2ba4b180cb69ca38, 0x6d16ee18552116dd,
	0x0febd845d0dfae43, 0xc87a746319c16a0d, 0xdaf66c5f2577bffa, 0x38018b47b29a8b06,
	0xd12ecbc40b9475b1, 0xa25b59fd92e8e269, 0xefbfc19ee8f6cf32, 0x9a27d85888c132ad,
	0x12f175ffae3b16ec, 0x1fdb8b3206d599e8, 0x3042e325a28f5ab0, 0xd480865f9b38fe80,
	0x1ea45cd69371a71f, 0x176ea1b164264cd5, 0xd576d4155ec17dbe, 0x1db53334fb0323a1,
	0x9b0252440950fd13, 0x31d0b6640589f877, 0xf87f43fdf6062541, 0xb7d6467b2f5a522a,
	0x7aaf0e891fb797fa, 0xba26d85135e8579a, 0x0fa34266ccfdba9b, 0xade9b2b4efdd35f8,
	0x8b53031d05d51433, 0x9edfa3da6cf55b15, 0xd5fdb76a19fbeb1d, 0x11ebcd49428a1c22,
	0x126cbc8f38884479, 0x4d125e7fa59cec98, 0x6fa231e959acdd98, 0x0fa07a3f2e295065,
	0x7795e98680ee526e, 0x98b33c6e0a14b90a, 0xb306d70019d5f970, 0x642aad48fcfcfa81,
	0x429817c53308fb2e, 0xe786ab375bca47be, 0x78601602bb4a06cb, 0xe6fd68e8d69c91c2,
	0x91dc59efeb21a3f6, 0xb29c467d2b5f6932, 0x3412fc12ac322c12, 0xc470f0e7f76fbfb8,
	0xc9e4dab20edc6d2b, 0x28805c5dad1b8f60, 0x2975d279d86dbf11, 0x878b9f6b57a1cb71,
	0x1e01a934402d0baf, 0xebe2136898c75205, 0xaa6524ab713b7e05, 0x0361524c2cc0f859,
	0xae68690a78bc7175, 0xe66cd36e68ef8f5f, 0xdff3334b91b15f5d, 0xeae2025e82339e23,
	0xa62081434fbaecc0, 0x637e0edc5b6e4ae7, 0xa859890cd670f668, 0x27460f22403d1f83,
	0xb0d9c2aa8f837ef7, 0x753c7c99032f06ca, 0x143e2e04bdd7d19b, 0xbd30291a55fea08e,
	0x8b5885ca0bb2c3f0, 0x2284b7a447e7f593, 0xc31d5a973d792fa1, 0x7b59051bf40048d7,
	0x9c31d9b25a2b745b, 0xac642b4c49b25ded, 0x971c702d5bf49c04, 0xe456697cf2686baa,
	0xda90f534a23d4c9d, 0x21e150949efee464, 0x4f6fa985b732d46f, 0xbf9cc545635518f7,
	0xd432f8db6a174c1c, 0x14aa451ca69cfb85, 0x983631890063e42f, 0xb2d650af313b32b7,
	0x28fafd04559b5975, 0x391cf0463d4a5d51, 0x72b8ff39a32c9b6f, 0xb5d97ef760ef1471,
	0xac7c8803e01bbf50, 0xdfe1b30791725f0a, 0x08135d586a1689ad, 0xdf26f51766faf989,
	0x9145de05b3ab1b2c, 0xc5adf6816b10e53a, 0xb5816b74a985ab61, 0x2a69acc70bf9c0ef,
	0x105ada6b720299e3, 0xb3969057425cb200, 0x7244f536285e25b4, 0xe28bc9ff870f084c,
	0xe8754cd37cbd7025, 0x9a9e43108fb83bab, 0x0004884cc167733f, 0x09f6048fe245a460,
	0x53710f577e9cf84f, 0xd675ebf74fe30c9a, 0x0cc36d8c77863fe5, 0xd29dc5dfcf1da110,
	0xf963a7efe00111e5, 0x6a46721acffa6cdd, 0x8c6e90373020da5c, 0xf689a4a5ffda0336,
	0xfa83ada4a2121ac5, 0xd663049d155e18b1, 0x2169df82b9bdee2d, 0x03c54c71fca05536,
	0xf3158c0c66dd7794, 0x6ae04d52adb328cb, 0x00de59f550f0fc2b, 0x03a8987936a98d74,
	0xc1378be5b7a28e0a, 0xfaf1501b009a815b, 0xacfebb4bd29e8693, 0x9cb017c18741ae91,
	0x30c1fb6a19086515, 0x9bbd750d1e707c52, 0x32d1f81ba636425c, 0x4d6b234fdfa7c6ed,
	0xb044284a47acf2f6, 0x2ea60b99fa7ff8bf, 0x79c147c719a5711b, 0xec3aa314da9bb017,
	0xa0acf4c9658de17e, 0x0597aab614d30dbc, 0xe9f41cc04653a560, 0xccc14d5173f660d8,
	0x1da3b7e2cad6e514, 0x41a93f90dc821527, 0xa7502a812227d96d, 0xd138d1508557716a,
	0xa51ad4f3a699bae0, 0x1d77ce4058d87776, 0x27896389df3277fd, 0xd9ead9264745dd9e,
	0x0ad4041504c14982, 0x34ab18fd0a68e88e, 0x4279b14dae55cdff, 0x50910bdc8ef066d4,
	0x5decc06af24dfdd8, 0x914591aef03d866a, 0xd974c146e8ec01b3, 0xd8ab0b300ac0cf0d,
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
