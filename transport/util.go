package transport

import (
	"crypto/tls"
	"strconv"
)

// TLSVersionString returns a string representation of the TLS version number.
func TLSVersionString(v uint16) string {
	switch v {
	case tls.VersionTLS10:
		return "1.0"
	case tls.VersionTLS11:
		return "1.1"
	case tls.VersionTLS12:
		return "1.2"
	case tls.VersionTLS13:
		return "1.3"
	default:
		if v == 0 {
			return "unknown"
		}
		return strconv.FormatUint(uint64(v), 10)
	}
}
