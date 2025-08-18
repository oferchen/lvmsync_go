package logging

import (
	"crypto/tls"
	"strconv"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	config "lvmsync_go/internal/config"
)

// Option configures NewLogger behavior.
type Option func(*options)

type options struct {
	samplingFirst      int
	samplingThereafter int
	redactor           func(zapcore.Field) zapcore.Field
	zapOpts            []zap.Option
}

func defaultOptions() options {
	return options{
		samplingFirst:      100,
		samplingThereafter: 100,
	}
}

func applyOptions(opts []Option) options {
	o := defaultOptions()
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

// WithSampling sets the sampling rates. A zero value disables sampling.
func WithSampling(first, thereafter int) Option {
	return func(o *options) {
		o.samplingFirst = first
		o.samplingThereafter = thereafter
	}
}

// WithRedactHook registers a field redaction hook applied to all log fields.
func WithRedactHook(fn func(zapcore.Field) zapcore.Field) Option {
	return func(o *options) { o.redactor = fn }
}

// WithZapOptions appends zap.Options used when building the logger.
func WithZapOptions(zopts ...zap.Option) Option {
	return func(o *options) { o.zapOpts = append(o.zapOpts, zopts...) }
}

type redactingCore struct {
	zapcore.Core
	redactor func(zapcore.Field) zapcore.Field
}

func (c redactingCore) With(fields []zapcore.Field) zapcore.Core {
	if c.redactor != nil {
		rf := make([]zapcore.Field, len(fields))
		for i, f := range fields {
			rf[i] = c.redactor(f)
		}
		return redactingCore{Core: c.Core.With(rf), redactor: c.redactor}
	}
	return redactingCore{Core: c.Core.With(fields), redactor: c.redactor}
}

func (c redactingCore) Check(ent zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.Enabled(ent.Level) {
		return ce.AddCore(ent, c)
	}
	return ce
}

func (c redactingCore) Write(ent zapcore.Entry, fields []zapcore.Field) error {
	if c.redactor != nil {
		rf := make([]zapcore.Field, len(fields))
		for i, f := range fields {
			rf[i] = c.redactor(f)
		}
		return c.Core.Write(ent, rf)
	}
	return c.Core.Write(ent, fields)
}

// NewLogger builds a production logger with sampling enabled, adds caller
// information, annotates logs with the provided component name, and
// applies optional redaction. Debug level is set when cfg.Verbose > 0.
// Sampling rates and redaction hooks can be customized via opts.
func NewLogger(cfg *config.Config, component string, opts ...Option) (*zap.Logger, error) {
	o := applyOptions(opts)
	c := zap.NewProductionConfig()
	if cfg != nil && cfg.Verbose > 0 {
		c.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
	}
	if o.samplingFirst == 0 || o.samplingThereafter == 0 {
		c.Sampling = nil
	} else {
		c.Sampling = &zap.SamplingConfig{Initial: int(o.samplingFirst), Thereafter: int(o.samplingThereafter)}
	}
	zapOpts := []zap.Option{zap.AddCaller()}
	if o.redactor != nil {
		zapOpts = append(zapOpts, zap.WrapCore(func(core zapcore.Core) zapcore.Core {
			return redactingCore{Core: core, redactor: o.redactor}
		}))
	}
	zapOpts = append(zapOpts, o.zapOpts...)
	zapOpts = append(zapOpts, zap.Fields(zap.String("component", component)))
	return c.Build(zapOpts...)
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
