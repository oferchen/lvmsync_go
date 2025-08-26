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

// SnapshotRegistry tracks LVM snapshots for cleanup on process exit.
type SnapshotRegistry struct {
	mu          sync.Mutex
	registry    map[string]*zap.Logger
	handlerOnce sync.Once
	remove      func(context.Context, string, *zap.Logger) error
}

// NewSnapshotRegistry constructs a SnapshotRegistry. If remove is nil,
// RemoveSnapshot is used.
func NewSnapshotRegistry(remove func(context.Context, string, *zap.Logger) error) *SnapshotRegistry {
	if remove == nil {
		remove = RemoveSnapshot
	}
	return &SnapshotRegistry{
		registry: make(map[string]*zap.Logger),
		remove:   remove,
	}
}

// RegisterSnapshot adds the snapshot path to the cleanup registry and installs
// a signal handler on first use.
func (r *SnapshotRegistry) RegisterSnapshot(path string, logger *zap.Logger) {
	if path == "" {
		return
	}
	r.mu.Lock()
	r.registry[path] = logger
	r.mu.Unlock()
	r.handlerOnce.Do(func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
		go func() {
			<-ch
			r.CleanupRegistered(context.Background())
			signal.Stop(ch)
		}()
	})
}

// UnregisterSnapshot removes the snapshot from the cleanup registry.
func (r *SnapshotRegistry) UnregisterSnapshot(path string) {
	r.mu.Lock()
	delete(r.registry, path)
	r.mu.Unlock()
}

// CleanupRegistered removes all registered snapshots. It is safe to call from a
// deferred function to ensure cleanup on panic or normal exit.
func (r *SnapshotRegistry) CleanupRegistered(ctx context.Context) {
	r.mu.Lock()
	paths := make([]string, 0, len(r.registry))
	loggers := make([]*zap.Logger, 0, len(r.registry))
	for p, l := range r.registry {
		paths = append(paths, p)
		loggers = append(loggers, l)
	}
	r.registry = make(map[string]*zap.Logger)
	r.mu.Unlock()
	if ctx == nil {
		ctx = context.Background()
	}
	for i, p := range paths {
		rmCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		if err := r.remove(rmCtx, p, loggers[i]); err != nil {
			loggers[i].Warn("failed to remove snapshot", zap.String("snapshot", p), zap.Error(err))
		} else {
			loggers[i].Info("snapshot removed", zap.String("snapshot", p))
		}
		cancel()
	}
}

// CleanupSnapshot removes a single snapshot, unregistering it first. It logs
// success or failure and ignores empty paths.
func (r *SnapshotRegistry) CleanupSnapshot(ctx context.Context, path string, logger *zap.Logger) {
	if path == "" {
		return
	}
	r.UnregisterSnapshot(path)
	if ctx == nil {
		ctx = context.Background()
	}
	rmCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := r.remove(rmCtx, path, logger); err != nil {
		logger.Warn("failed to remove snapshot", zap.String("snapshot", path), zap.Error(err))
	} else {
		logger.Info("snapshot removed", zap.String("snapshot", path))
	}
}
