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
// of a transfer. It is a simple space separated list of key/value tokens that
// mirrors rsync's textual negotiation.
type Handshake struct {
	Version     string
	Transports  []string
	Compressors []string
	Digests     []string

	Transport     string
	Compress      string
	CompressLevel int
	Digest        string

	Checksum      bool
	ChecksumDedup bool
	CRC32C        bool
	Endianness    string
	BlockSize     int
	DedupMode     string
	ResumeToken   string
	ODirect       bool
	MaxInFlight   int

	CDCMin int
	CDCAvg int
	CDCMax int

	ALPN       string
	TLSVersion string
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

// String reconstructs the textual representation of the handshake. It mirrors
// the line emitted by WriteHandshake without the trailing newline.
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
	if h.MaxInFlight > 0 {
		tokens = append(tokens, fmt.Sprintf("inflight:%d", h.MaxInFlight))
	}
	if h.CDCMin > 0 {
		tokens = append(tokens, fmt.Sprintf("cdcmin:%d", h.CDCMin))
	}
	if h.CDCAvg > 0 {
		tokens = append(tokens, fmt.Sprintf("cdcavg:%d", h.CDCAvg))
	}
	if h.CDCMax > 0 {
		tokens = append(tokens, fmt.Sprintf("cdcmax:%d", h.CDCMax))
	}
	if h.ALPN != "" {
		tokens = append(tokens, "alpn:"+h.ALPN)
	}
	if h.TLSVersion != "" {
		tokens = append(tokens, "tls:"+h.TLSVersion)
	}

	if len(h.Transports) > 0 {
		tokens = append(tokens, "transports:"+strings.Join(h.Transports, ","))
	}
	if len(h.Compressors) > 0 {
		tokens = append(tokens, "compressors:"+strings.Join(h.Compressors, ","))
	}
	if len(h.Digests) > 0 {
		tokens = append(tokens, "digests:"+strings.Join(h.Digests, ","))
	}

	if h.CRC32C {
		tokens = append(tokens, "crc32c")
	}
	if h.ChecksumDedup {
		tokens = append(tokens, "checksum-dedup")
	} else if h.Checksum {
		tokens = append(tokens, "checksum")
	}

	if h.Transport != "" {
		tokens = append(tokens, "transport:"+h.Transport)
	}
	if h.Compress == "" {
		h.Compress = "none"
	}
	tokens = append(tokens, "compress:"+h.Compress)
	if h.Digest != "" {
		tokens = append(tokens, "digest:"+h.Digest)
	}
	if h.CompressLevel != 0 {
		tokens = append(tokens, fmt.Sprintf("level:%d", h.CompressLevel))
	}

	if _, err := fmt.Fprintln(w, strings.Join(tokens, " ")); err != nil {
		return fmt.Errorf("write handshake: %w", err)
	}
	return nil
}

// ReadHandshake parses a handshake from r.
// r must be a bufio.Reader so the caller can continue reading the remaining
// stream after the handshake has been consumed.
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
			h.Transport = strings.TrimPrefix(t, "transport:")
		case strings.HasPrefix(t, "transports:"):
			h.Transports = splitNonEmpty(strings.TrimPrefix(t, "transports:"))
		case strings.HasPrefix(t, "compress:"):
			h.Compress = strings.TrimPrefix(t, "compress:")
		case strings.HasPrefix(t, "compressors:"):
			h.Compressors = splitNonEmpty(strings.TrimPrefix(t, "compressors:"))
		case strings.HasPrefix(t, "digest:"):
			h.Digest = strings.TrimPrefix(t, "digest:")
		case strings.HasPrefix(t, "digests:"):
			h.Digests = splitNonEmpty(strings.TrimPrefix(t, "digests:"))
		case strings.HasPrefix(t, "level:"):
			lvl, err := strconv.Atoi(strings.TrimPrefix(t, "level:"))
			if err != nil {
				return Handshake{}, fmt.Errorf("invalid compression level: %w", err)
			}
			h.CompressLevel = lvl
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
		case strings.HasPrefix(t, "inflight:"):
			m, err := strconv.Atoi(strings.TrimPrefix(t, "inflight:"))
			if err != nil {
				return Handshake{}, fmt.Errorf("invalid max in-flight: %w", err)
			}
			h.MaxInFlight = m
		case strings.HasPrefix(t, "cdcmin:"):
			v, err := strconv.Atoi(strings.TrimPrefix(t, "cdcmin:"))
			if err != nil {
				return Handshake{}, fmt.Errorf("invalid cdc min: %w", err)
			}
			h.CDCMin = v
		case strings.HasPrefix(t, "cdcavg:"):
			v, err := strconv.Atoi(strings.TrimPrefix(t, "cdcavg:"))
			if err != nil {
				return Handshake{}, fmt.Errorf("invalid cdc avg: %w", err)
			}
			h.CDCAvg = v
		case strings.HasPrefix(t, "cdcmax:"):
			v, err := strconv.Atoi(strings.TrimPrefix(t, "cdcmax:"))
			if err != nil {
				return Handshake{}, fmt.Errorf("invalid cdc max: %w", err)
			}
			h.CDCMax = v
		case strings.HasPrefix(t, "alpn:"):
			h.ALPN = strings.TrimPrefix(t, "alpn:")
		case strings.HasPrefix(t, "tls:"):
			h.TLSVersion = strings.TrimPrefix(t, "tls:")
		case t == "crc32c":
			h.CRC32C = true
		case t == "checksum":
			h.Checksum = true
		case t == "checksum-dedup":
			h.Checksum = true
			h.ChecksumDedup = true
		}
	}
	if h.Compress == "" {
		h.Compress = "none"
	}
	return h, nil
}

// SelectBest returns the first element from local that is also present in
// remote. If no common element exists, the first element of local is returned or
// an empty string if local is empty.
func SelectBest(local, remote []string) string {
	if len(local) == 0 {
		return ""
	}
	if len(remote) == 0 {
		return local[0]
	}

	remoteSet := make(map[string]struct{}, len(remote))
	for _, r := range remote {
		remoteSet[r] = struct{}{}
	}
	for _, l := range local {
		if _, ok := remoteSet[l]; ok {
			return l
		}
	}
	return local[0]
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
