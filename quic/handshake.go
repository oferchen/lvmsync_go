package quic

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// Negotiation describes parameters exchanged during a QUIC handshake.
type Negotiation struct {
	Protocol  string
	Algorithm string
	TestSpace string
}

// WriteNegotiation serializes n to w.
func WriteNegotiation(w io.Writer, n Negotiation) error {
	tokens := []string{n.Protocol}
	if n.Algorithm != "" {
		tokens = append(tokens, "algo:"+n.Algorithm)
	}
	if n.TestSpace != "" {
		tokens = append(tokens, "test:"+n.TestSpace)
	}
	if _, err := fmt.Fprintln(w, strings.Join(tokens, " ")); err != nil {
		return fmt.Errorf("write negotiation: %w", err)
	}
	return nil
}

// ReadNegotiation parses a negotiation from r.
func ReadNegotiation(r *bufio.Reader) (Negotiation, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return Negotiation{}, fmt.Errorf("read negotiation: %w", err)
	}
	parts := strings.Fields(strings.TrimSpace(line))
	if len(parts) == 0 {
		return Negotiation{}, fmt.Errorf("empty negotiation")
	}
	n := Negotiation{Protocol: parts[0]}
	for _, p := range parts[1:] {
		switch {
		case strings.HasPrefix(p, "algo:"):
			n.Algorithm = strings.TrimPrefix(p, "algo:")
		case strings.HasPrefix(p, "test:"):
			n.TestSpace = strings.TrimPrefix(p, "test:")
		default:
			return Negotiation{}, fmt.Errorf("unexpected token %s", p)
		}
	}
	return n, nil
}

// Negotiate performs a negotiation on rw and echoes the parameters back.
func Negotiate(rw io.ReadWriter, expected Negotiation) error {
	br := bufio.NewReader(rw)
	n, err := ReadNegotiation(br)
	if err != nil {
		return err
	}
	if n.Protocol != expected.Protocol || n.Algorithm != expected.Algorithm || n.TestSpace != expected.TestSpace {
		return fmt.Errorf("negotiation mismatch")
	}
	return WriteNegotiation(rw, expected)
}
