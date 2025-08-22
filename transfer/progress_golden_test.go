package transfer

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/oferchen/lvmsync_go/internal/config"
)

func TestProgressLogGolden(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	cfg := &config.Config{Progress: true}
	reportProgress(cfg, 50, 100, 1, time.Now(), logger)
	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(entries))
	}
	entry := entries[0]
	encCfg := zapcore.EncoderConfig{
		MessageKey:  "msg",
		LevelKey:    "level",
		EncodeLevel: zapcore.LowercaseLevelEncoder,
	}
	enc := zapcore.NewJSONEncoder(encCfg)
	buf, err := enc.EncodeEntry(entry.Entry, entry.Context)
	if err != nil {
		t.Fatalf("encode entry: %v", err)
	}
	got := buf.Bytes()
	want, err := os.ReadFile("testdata/progress_event.json")
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	var gotMap, wantMap map[string]any
	if err := json.Unmarshal(got, &gotMap); err != nil {
		t.Fatalf("unmarshal got: %v", err)
	}
	if err := json.Unmarshal(want, &wantMap); err != nil {
		t.Fatalf("unmarshal want: %v", err)
	}
	if !reflect.DeepEqual(gotMap, wantMap) {
		t.Fatalf("log entry mismatch\n got: %v\nwant: %v", gotMap, wantMap)
	}
}
