package logging

import (
	"reflect"
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	config "github.com/oferchen/lvmsync_go/internal/config"
)

func TestNewLoggerSamplingDefaults(t *testing.T) {
	logger, err := NewLogger(&config.Config{}, "test")
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	core := logger.Core()
	typ := reflect.TypeOf(core).String()
	if !strings.Contains(typ, "sampler") {
		t.Fatalf("core type %s, want sampler", typ)
	}
	rv := reflect.ValueOf(core).Elem()
	first := rv.FieldByName("first").Uint()
	thereafter := rv.FieldByName("thereafter").Uint()
	if first != 100 || thereafter != 100 {
		t.Fatalf("sampling defaults first=%d thereafter=%d", first, thereafter)
	}
}

func TestNewLoggerVerboseLevel(t *testing.T) {
	logger, err := NewLogger(&config.Config{Verbose: 1}, "test")
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	if !logger.Core().Enabled(zap.DebugLevel) {
		t.Fatalf("logger level %v, want debug", zap.DebugLevel)
	}
}

func TestNewLoggerRedactionAndComponent(t *testing.T) {
	redactor := func(f zapcore.Field) zapcore.Field {
		if f.Key == "secret" && f.Type == zapcore.StringType {
			f.String = "[REDACTED]"
		}
		return f
	}
	core, logs := observer.New(zap.InfoLevel)
	logger, err := NewLogger(
		&config.Config{},
		"comp",
		WithSampling(0, 0),
		WithRedactHook(redactor),
		WithZapOptions(zap.WrapCore(func(c zapcore.Core) zapcore.Core {
			return zapcore.NewTee(c, redactingCore{Core: core, redactor: redactor})
		})),
	)
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	logger.Info("msg", zap.String("secret", "token"))
	if logs.Len() != 1 {
		t.Fatalf("log entries %d, want 1", logs.Len())
	}
	fields := logs.All()[0].ContextMap()
	if v := fields["secret"]; v != "[REDACTED]" {
		t.Fatalf("secret field %v, want [REDACTED]", v)
	}
	if v := fields["component"]; v != "comp" {
		t.Fatalf("component field %v, want comp", v)
	}
}

func TestRedactingCoreWith(t *testing.T) {
	redactor := func(f zapcore.Field) zapcore.Field {
		if f.Key == "private_key" && f.Type == zapcore.StringType {
			f.String = "[REDACTED]"
		}
		return f
	}
	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(redactingCore{Core: core, redactor: redactor}).With(zap.String("private_key", "secret"))
	logger.Info("msg")
	if logs.Len() != 1 {
		t.Fatalf("log entries %d, want 1", logs.Len())
	}
	fields := logs.All()[0].ContextMap()
	if v := fields["private_key"]; v != "[REDACTED]" {
		t.Fatalf("private_key field %v, want [REDACTED]", v)
	}
}
