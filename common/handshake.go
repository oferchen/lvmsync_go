package common

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// Handshake describes protocol negotiation parameters exchanged at the start
// of a transfer. The format is a single line of space separated tokens:
//
//	lvmsync PROTO[3] compress:<algo> [checksum|checksum-dedup]
//
// Additional tokens may be added in the future while preserving backward
// compatibility. The receiver must ignore unknown tokens to allow for
// extension.
//
// Compress specifies the compression algorithm in use. Checksum indicates
// whether chunk checksums are included. When ChecksumDedup is true the
// checksum list also doubles as a deduplication map.
//
// Version will always be set to ProtocolVersion on successful parsing.
//
// Handshake is deliberately simple to mirror rsync's textual negotiation
// while remaining easy to extend and debug.
//
// This package aims to centralize handshake formatting and parsing to keep
// transfer/ code focused on business logic and improve maintainability.
type Handshake struct {
	Version       string
	Compress      string
	Checksum      bool
	ChecksumDedup bool
}

// String reconstructs the textual representation of the handshake. It is
// primarily intended for diagnostics and mirrors the line emitted by
// WriteHandshake without the trailing newline.
func (h Handshake) String() string {
	var sb strings.Builder
	_ = WriteHandshake(&sb, h) // Writing to strings.Builder never fails.
	return strings.TrimSpace(sb.String())
}

// WriteHandshake serializes h to w using the protocol line format. A trailing
// newline is always written.
func WriteHandshake(w io.Writer, h Handshake) error {
	tokens := []string{ProtocolVersion}
	if h.ChecksumDedup {
		tokens = append(tokens, "checksum-dedup")
	} else if h.Checksum {
		tokens = append(tokens, "checksum")
	}
	if h.Compress == "" {
		h.Compress = "none"
	}
	tokens = append(tokens, "compress:"+h.Compress)
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
	h := Handshake{Version: ProtocolVersion, Compress: "none"}
	for _, t := range strings.Fields(rest) {
		switch {
		case strings.HasPrefix(t, "compress:"):
			h.Compress = strings.TrimPrefix(t, "compress:")
		case t == "checksum":
			h.Checksum = true
		case t == "checksum-dedup":
			h.Checksum = true
			h.ChecksumDedup = true
		default:
			return Handshake{}, fmt.Errorf("unexpected token in handshake: %s", t)
		}
	}
	return h, nil
}
