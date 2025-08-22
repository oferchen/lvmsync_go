package lvm

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"go.uber.org/zap"
)

var (
	registryMu  sync.Mutex
	registry    = make(map[string]*zap.Logger)
	handlerOnce sync.Once
	removeSnap  = RemoveSnapshot
)

// RegisterSnapshot adds the snapshot path to the cleanup registry and installs
// a signal handler on first use. If logger is nil, zap.NewNop() is used.
func RegisterSnapshot(path string, logger *zap.Logger) {
	if path == "" {
		return
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	registryMu.Lock()
	registry[path] = logger
	registryMu.Unlock()
	handlerOnce.Do(func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
		go func() {
			<-ch
			CleanupRegistered(context.Background())
			signal.Stop(ch)
		}()
	})
}

// UnregisterSnapshot removes the snapshot from the cleanup registry.
func UnregisterSnapshot(path string) {
	registryMu.Lock()
	delete(registry, path)
	registryMu.Unlock()
}

// CleanupRegistered removes all registered snapshots. It is safe to call from a
// deferred function to ensure cleanup on panic or normal exit.
func CleanupRegistered(ctx context.Context) {
	registryMu.Lock()
	paths := make([]string, 0, len(registry))
	loggers := make([]*zap.Logger, 0, len(registry))
	for p, l := range registry {
		paths = append(paths, p)
		loggers = append(loggers, l)
	}
	registry = make(map[string]*zap.Logger)
	registryMu.Unlock()
	if ctx == nil {
		ctx = context.Background()
	}
	for i, p := range paths {
		rmCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		if err := removeSnap(rmCtx, p, loggers[i]); err != nil {
			loggers[i].Warn("failed to remove snapshot", zap.String("snapshot", p), zap.Error(err))
		} else {
			loggers[i].Info("snapshot removed", zap.String("snapshot", p))
		}
		cancel()
	}
}

// CleanupSnapshot removes a single snapshot, unregistering it first. If logger
// is nil, zap.NewNop() is used. It logs success or failure and ignores empty
// paths.
func CleanupSnapshot(ctx context.Context, path string, logger *zap.Logger) {
	if path == "" {
		return
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	UnregisterSnapshot(path)
	if ctx == nil {
		ctx = context.Background()
	}
	rmCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := removeSnap(rmCtx, path, logger); err != nil {
		logger.Warn("failed to remove snapshot", zap.String("snapshot", path), zap.Error(err))
	} else {
		logger.Info("snapshot removed", zap.String("snapshot", path))
	}
}
