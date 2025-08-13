package serve

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"lvmsync_go/config"
)

type bufferSyncer struct {
	buf     bytes.Buffer
	synced  bool
	syncErr error
}

func (b *bufferSyncer) Write(p []byte) (int, error) {
	return b.buf.Write(p)
}

func (b *bufferSyncer) Sync() error {
	b.synced = true
	return b.syncErr
}

func newLogger(bs *bufferSyncer) *zap.Logger {
	encCfg := zap.NewProductionEncoderConfig()
	core := zapcore.NewCore(zapcore.NewJSONEncoder(encCfg), zapcore.AddSync(bs), zapcore.DebugLevel)
	return zap.New(core)
}

func TestRunCancelsContextAndFlushesLogs(t *testing.T) {
	bs := &bufferSyncer{}
	logger := newLogger(bs)
	ctx, err := Run(context.Background(), &config.Config{}, logger)
	if err == nil {
		t.Fatalf("expected error")
	}
	select {
	case <-ctx.Done():
	default:
		t.Fatalf("expected context to be cancelled")
	}
	if !bs.synced {
		t.Fatalf("expected logger.Sync to be called")
	}
	if !strings.Contains(bs.buf.String(), "serve mode not implemented") {
		t.Fatalf("log not flushed: %s", bs.buf.String())
	}
}

func TestRunLogsSyncError(t *testing.T) {
	bs := &bufferSyncer{syncErr: errors.New("sync fail")}
	logger := newLogger(bs)
	if _, err := Run(context.Background(), &config.Config{}, logger); err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(bs.buf.String(), "sync_error") {
		t.Fatalf("expected sync error to be logged: %s", bs.buf.String())
	}
}
