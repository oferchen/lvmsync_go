package logging

import (
	"crypto/tls"
	"strconv"

	"go.uber.org/zap"

	config "lvmsync_go/internal/config"
)

// NewLogger builds a production logger with sampling enabled and
// sets debug level when cfg.Verbose > 0.
func NewLogger(cfg *config.Config, opts ...zap.Option) (*zap.Logger, error) {
	c := zap.NewProductionConfig()
	if cfg != nil && cfg.Verbose > 0 {
		c.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
	}
	return c.Build(opts...)
}

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

package logging

import (
	"crypto/tls"
	"strconv"

	"go.uber.org/zap"

	"lvmsync_go/internal/config"
)
