package logging

import (
	"reflect"
	"strings"
	"testing"

	"go.uber.org/zap"

	config "lvmsync_go/internal/config"
)

func TestNewLoggerSamplingDefaults(t *testing.T) {
	logger, err := NewLogger(&config.Config{})
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
	logger, err := NewLogger(&config.Config{Verbose: 1})
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	if !logger.Core().Enabled(zap.DebugLevel) {
		t.Fatalf("logger level %v, want debug", zap.DebugLevel)
	}
}
