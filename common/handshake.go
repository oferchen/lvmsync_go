package common //nolint:revive // package name is established API

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unsafe"
)

// Handshake describes protocol negotiation parameters exchanged at the start
// of a transfer. The format is a single line of space separated tokens:
//
//	lvmsync PROTO[3] [endian:<little|big>] [block:<bytes>]
//	        [dedup:<fixed|cdc|hybrid>] [resume:<token>] [odirect]
//	        [checksum|checksum-dedup] compress:<algo> [level:<n>]
//
// Additional tokens may be added in the future while preserving backward
// compatibility. The receiver must ignore unknown tokens to allow for
// extension.
//
// Compress specifies the compression algorithm in use. Checksum indicates
// whether chunk checksums are included. When ChecksumDedup is true the
// checksum list also doubles as a deduplication map. Endianness advertises the
// sender's byte order, BlockSize conveys the preferred chunk size, DedupMode
// announces the deduplication strategy, ResumeToken resumes interrupted
// transfers, and ODirect signals support for `O_DIRECT` I/O.
//
// Version will always be set to ProtocolVersion on successful parsing.
//
// Handshake is deliberately simple to mirror rsync's textual negotiation
// while remaining easy to extend and debug.
//
// This package aims to centralize handshake formatting and parsing to keep
// transfer/ code focused on business logic and improve maintainability.
// Handshake represents the negotiated parameters between peers.
type Handshake struct {
	Version       string
	Transports    []string
	Compress      []string
	CompressLevel int
	Digests       []string
	Checksum      bool
	ChecksumDedup bool
	Endianness    string
	BlockSize     int
	DedupMode     string
	ResumeToken   string
	ODirect       bool
}

// NativeEndianness reports the platform byte order as "little" or "big".
func NativeEndianness() string {
	var i uint16 = 1
	b := (*[2]byte)(unsafe.Pointer(&i))
	if b[0] == 1 {
		return "little"
	}
	return "big"
}

// String reconstructs the textual representation of the handshake. It is
// primarily intended for diagnostics and mirrors the line emitted by
// WriteHandshake without the trailing newline.
func (h Handshake) String() string {
	var sb strings.Builder
	if err := WriteHandshake(&sb, h); err != nil {
		return ""
	}
	return strings.TrimSpace(sb.String())
}

// WriteHandshake serializes h to w using the protocol line format. A trailing
// newline is always written.
func WriteHandshake(w io.Writer, h Handshake) error {
	tokens := []string{ProtocolVersion}

	if h.Endianness != "" {
		tokens = append(tokens, "endian:"+h.Endianness)
	}
	if h.BlockSize > 0 {
		tokens = append(tokens, fmt.Sprintf("block:%d", h.BlockSize))
	}
	if h.DedupMode != "" {
		tokens = append(tokens, "dedup:"+h.DedupMode)
	}
	if h.ResumeToken != "" {
		tokens = append(tokens, "resume:"+h.ResumeToken)
	}
	if h.ODirect {
		tokens = append(tokens, "odirect")
	}

	if h.ChecksumDedup {
		tokens = append(tokens, "checksum-dedup")
	} else if h.Checksum {
		tokens = append(tokens, "checksum")
	}
	if len(h.Transports) > 0 {
		tokens = append(tokens, "transport:"+strings.Join(h.Transports, ","))
	}
	if len(h.Compress) == 0 {
		h.Compress = []string{"none"}
	}
	tokens = append(tokens, "compress:"+strings.Join(h.Compress, ","))
	if len(h.Digests) > 0 {
		tokens = append(tokens, "digest:"+strings.Join(h.Digests, ","))
	}
	tokens = append(tokens, "compress:"+h.Compress)
	if h.CompressLevel != 0 {
		tokens = append(tokens, fmt.Sprintf("level:%d", h.CompressLevel))
	}

	if _, err := fmt.Fprintln(w, strings.Join(tokens, " ")); err != nil {
		return fmt.Errorf("write handshake: %w", err)
	}
	return nil
}

// ReadHandshake parses a handshake from r. r must be a bufio.Reader so that
// the caller can continue reading the remaining stream after the handshake
// has been consumed.
func ReadHandshake(r *bufio.Reader) (Handshake, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return Handshake{}, fmt.Errorf("read handshake: %w", err)
	}
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, ProtocolVersion) {
		return Handshake{}, fmt.Errorf("unexpected protocol handshake: %s", line)
	}
	rest := strings.TrimSpace(strings.TrimPrefix(line, ProtocolVersion))
	h := Handshake{Version: ProtocolVersion}
	for _, t := range strings.Fields(rest) {
		switch {
		case strings.HasPrefix(t, "transport:"):
			h.Transports = splitNonEmpty(strings.TrimPrefix(t, "transport:"))
		case strings.HasPrefix(t, "compress:"):
			h.Compress = splitNonEmpty(strings.TrimPrefix(t, "compress:"))
		case strings.HasPrefix(t, "digest:"):
			h.Digests = splitNonEmpty(strings.TrimPrefix(t, "digest:"))
		case strings.HasPrefix(t, "endian:"):
			h.Endianness = strings.TrimPrefix(t, "endian:")
		case strings.HasPrefix(t, "block:"):
			bs, err := strconv.Atoi(strings.TrimPrefix(t, "block:"))
			if err != nil {
				return Handshake{}, fmt.Errorf("invalid block size: %w", err)
			}
			h.BlockSize = bs
		case strings.HasPrefix(t, "dedup:"):
			h.DedupMode = strings.TrimPrefix(t, "dedup:")
		case strings.HasPrefix(t, "resume:"):
			h.ResumeToken = strings.TrimPrefix(t, "resume:")
		case t == "odirect":
			h.ODirect = true
		case t == "checksum":
			h.Checksum = true
		case t == "checksum-dedup":
			h.Checksum = true
			h.ChecksumDedup = true
		default:
			// Ignore unknown tokens to preserve forward compatibility.
		}
	}
	if len(h.Compress) == 0 {
		h.Compress = []string{"none"}
	}
	return h, nil
}

func splitNonEmpty(v string) []string {
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
