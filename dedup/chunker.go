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
// gear table generated using Python random seed 1 (FastCDC reference).
var gear = [256]uint64{
	0x91b7584a2265b1f5,
	0xcd613e30d8f16adf,
	0x1027c4d1c386bbc4,
	0x1e2feb89414c343c,
	0xc2ce6f447ed4d57b,
	0x78e510617311d8a3,
	0x612e7696a6cecc1b,
	0x35bf992dc9e9c616,
	0x7ce42c8218072e8c,
	0xe4b06ce60741c7a8,
	0x63ca828dd5f4b3b2,
	0x9b810e766ec9d286,
	0xc4647159c324c985,
	0xb2221a58008a05a6,
	0x442e3d437204e52d,
	0xcd447e35b8b6d8fe,
	0x9755d4c13a902931,
	0x1a2b8f1ff1fd42a2,
	0x51431193e6c3f339,
	0x05b6e6e307d4bedc,
	0xa648a7dd06839eb9,
	0x025b413f8a9a021e,
	0xe1988ad9f06c144a,
	0xafbd67f9619699cf,
	0xf8130c4237730edf,
	0xb9d179e06c0fd4f5,
	0x8712b8bc076f3787,
	0xc381e88f38c0c8fd,
	0xf06d3fef701966a0,
	0x8d88348a7eed8d14,
	0x587fd2803bab6c39,
	0xad45f23d3b1a11df,
	0xc2cd789a380208a9,
	0xf3c64af775a89294,
	0xed2f89d94a2f20aa,
	0x6a8ac4ba05805975,
	0xea90a8f0d66b829e,
	0xec148cb48e73ca47,
	0x19999e3fa46d6753,
	0xa11d459a2f978d87,
	0xb94067edfe175330,
	0x4be03db0dc2574bd,
	0xbe3edc0a1ef2a4f0,
	0xe5446dd4552b82f6,
	0xf9270f4eb8b333a8,
	0x803468b6b610a9f7,
	0xf79b17aeefba91fc,
	0x81f9c1f66c0f3459,
	0xe901e35cd47d380d,
	0x3099fdf5ab99254a,
	0x48beab134da98f1d,
	0xf9341c68966baea1,
	0x7fd63116e1ea24c4,
	0xf0dfb4a5d8a064df,
	0x64b2d2bc815a47c5,
	0xda71144896c8da19,
	0x7af027bc08d6af57,
	0xbe6521cc3e2434e3,
	0x677f6cbdcc22af58,
	0xaa2ca1af6a107b75,
	0x5dfbd3d12c4a3698,
	0xe1fab9d78c7e134f,
	0xc69d4bd8b3fa7aa7,
	0xbcfbb050acab1a6b,
	0x1622bd795fec898f,
	0xa9ec0806705fca16,
	0x1ba1621582283d15,
	0x29e821a4c74803e3,
	0xd707107e855c3844,
	0x5eda92d864ac5db9,
	0xbb968a437d5c8dfc,
	0x78255d6807923986,
	0x4efbc8d60b21fbac,
	0xd92a4aa2b410d93c,
	0x9d643c25fbb230bb,
	0x9403560d97dae38d,
	0xa5ac06d864c2f2e3,
	0x2b28fef02b9c014e,
	0x3a1890c78092b4d4,
	0x0326324dfb695ffb,
	0x33138131c541013d,
	0xeb8ac8ce8a245e6b,
	0x8c5fe8f8dc3bf364,
	0x678a5aa33b6fe507,
	0x5804f92283868a29,
	0xd8f33418f3d4e711,
	0x5a702cfa93ea5c4e,
	0xe8e5b4617589a82b,
	0xa8c24d4244ef7feb,
	0x9be3cecb8c497c68,
	0xbab9f87ff5059285,
	0x62397bc701762741,
	0xdb610487c89da11b,
	0xf463b337d20b5d59,
	0xf03edca7e2dcaa37,
	0x83333218bd91a1b7,
	0x21167d8fcf23cae8,
	0xc703806984c81999,
	0x349aae908fb5262c,
	0xf320cd576d14475b,
	0x7b297d0b0e5e18ba,
	0x5d5f576cdeb8fc4c,
	0x8ded3c9691eb79fa,
	0xf0e642f43328ad08,
	0x69d495dd81355c53,
	0xd037cdff7c240d49,
	0x6a17b9af5b569643,
	0x0067dba858989008,
	0x8a449ebe89d9bf02,
	0xc9546b439f9d0129,
	0x54c56c9a9cc9af4e,
	0x99901c0475491bc3,
	0xcdf8440407295e42,
	0xa2a7ae1f3ac7652c,
	0x8cfe5cd12d5db79b,
	0x2e47dc0e959f3a51,
	0x1773308cdc6b13ab,
	0x8d103ed3cc667e97,
	0xd9ed17e3cc0e95ee,
	0xee52bdb6d1020a15,
	0x084f3dd6415af341,
	0xf18dd1eed77c96c0,
	0x12093d26ac512b01,
	0xde3a5db5154ed512,
	0x73f7ba8e0445d656,
	0xc10faa4003ba33db,
	0x47fc816ac16e2284,
	0x44c5b4763fe31d03,
	0xcc1b0c3e1c07724e,
	0x2f429ce59ff3078f,
	0x4a5012dc582c18c9,
	0x2adf559a11cbc288,
	0x4155d7ef28dd37eb,
	0xf3b37f32870266c4,
	0xa81aa40a2b0b8c12,
	0xa5f09e6345ddb87d,
	0x4b63e0efb62ac1fe,
	0xb3df44a47467537a,
	0x7f1a355e526eb523,
	0x1d3b993f79490eab,
	0x4fdf8e1a060cea63,
	0x57e54acc62f5680c,
	0xcbd3f5e06bc15385,
	0x4227de213023580c,
	0x40e2a20a1bd7ce73,
	0xbaeb41a5e65a8149,
	0xfa0b85188296f5ea,
	0xf72f2bb83586fca7,
	0x6e80fa489b0bca16,
	0xf9bddea5d12982e4,
	0x39b21c95055455e8,
	0x65b675cd0492c4f5,
	0x090b20bb257e8454,
	0xf5bb9188b80599e9,
	0x721754ef2904acec,
	0x819d7ca7b46108cc,
	0x6d39eb43ad9cedde,
	0xd50e00978b7199cd,
	0xfa1b1bf13879399b,
	0xa17a4340f9c08fef,
	0xb1eedaffcc3d5506,
	0x736a947a843fdda7,
	0x861e02ec39235bc0,
	0x07dbf924a6048457,
	0xacc66a576518093d,
	0xcdaaac43936aa40c,
	0xa8ea37f7523d2a54,
	0x6d21f4cda185cc8e,
	0xbcc99ae80f0c8a89,
	0x202cc8284c717095,
	0x364e433ff7c882f4,
	0x0c250a03e023033d,
	0x121b28004e6f5a94,
	0x1391f9b9dbc799b0,
	0xeacc110e4f73fd94,
	0x4c41d9c0f07534fe,
	0x28804790be6c6fe9,
	0x909ff4976a8a43ef,
	0x21615022409a8a78,
	0x8f8b2b83022bc320,
	0xd9bc1d97e0f3a7ef,
	0x973082d609b4e5d2,
	0x37b4000bd1c51f86,
	0xe69bae29f652d008,
	0x75fa6dd891fde85c,
	0xd3f21dcc2be88b46,
	0xdeb0e066de26e655,
	0xc7af3626f9495568,
	0x9f7a7dafb43adc4f,
	0x0994940e82458cc8,
	0x334de73d60c290d0,
	0x1959b9ef58d07674,
	0x92c9357d34accd78,
	0xe585552fac954ab5,
	0x976699cc6ed5d1bf,
	0x7e0ab2ed31b1c27e,
	0xf01dbf291abb8ba3,
	0x63db01fcaa7c314b,
	0x810d2e304bcb6b22,
	0x04673b757ff2e341,
	0x9cb471a55349da48,
	0x66fec086df229650,
	0x4806aa81e65150b5,
	0x282ee0bc04a1bde4,
	0xdb87872d336b1a45,
	0xcfa6cf3e53e6d093,
	0x903715c8fcaf4a5a,
	0x2298bdb1c85f0d46,
	0x6de2b33b56cef8ec,
	0x443baac536891eeb,
	0x18ae013eaca91679,
	0x611575c2d67393d6,
	0x8c31406deea3d685,
	0xea190b2a58068a9d,
	0xd6730839e1e48557,
	0x88c9da8aafe673f6,
	0xc49872c67c081bb7,
	0x88534206fc4a447e,
	0x10b8fe223c116549,
	0x0a57af35b9b81635,
	0x220d672b15ad9a9d,
	0x2aa3300b2b711343,
	0x89c80c4de9367ed9,
	0x449c4ca23685156b,
	0x550d40ddc2557035,
	0x8181e84d99a74924,
	0x415ac400d7547080,
	0x56befa395e3c536c,
	0x1d296588571ceeee,
	0x3c35612e4a8d15d8,
	0xf1a9a658de0f39a7,
	0xc78fec459a9e994c,
	0xb7115c02f44d7e40,
	0x7d2186d3e323ce54,
	0x947810d822a608bf,
	0xc52f4fbe8d19821f,
	0x521b18a91ab1c42f,
	0x6816de060a04ef48,
	0x6156c4df12bccdcb,
	0xfdc1786bddbd358f,
	0x25b7501ac9c1ffef,
	0x20012170d418f7af,
	0x1d5c482557450e65,
	0x96605d959d7cd4f6,
	0xed192da3c82ad589,
	0x139f711060c73494,
	0x8cdece75921ebce6,
	0x90e32e8239455353,
	0xf3c668b114ed2049,
	0x5d698c8b44480030,
	0x4ba955f3e4096150,
	0x88c780f6907f9669,
	0x1d43d1ffecd1345e,
	0xe592067375305db7,
	0x1b943cfc46f57327,
	0x0bb662a8c979cb06,
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
