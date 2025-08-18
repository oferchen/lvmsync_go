package lvm

import (
	"container/list"
	"fmt"
	"sync"

	"go.uber.org/zap"
	"golang.org/x/sys/unix"
)

const fdCacheSize = 16

// fdCacheEntry represents a cached file descriptor.
type fdCacheEntry struct {
	path string
	fd   int
}

// FDCache provides a simple LRU cache for file descriptors.
type FDCache struct {
	fds    map[string]*list.Element
	order  *list.List
	mutex  sync.Mutex
	size   int
	logger *zap.Logger
}

// NewFDCache returns an initialized file descriptor cache with the given capacity.
func NewFDCache(size int, logger *zap.Logger) *FDCache {
	if size <= 0 {
		size = fdCacheSize
	}
	if logger == nil {
		panic("nil logger")
	}
	return &FDCache{
		fds:    make(map[string]*list.Element, size),
		order:  list.New(),
		size:   size,
		logger: logger,
	}
}

// NewDeviceFDCache returns an FDCache preconfigured for device descriptor caching.
func NewDeviceFDCache(logger *zap.Logger) *FDCache {
	return NewFDCache(fdCacheSize, logger)
}

// SetLogger updates the logger used by the cache.
func (c *FDCache) SetLogger(logger *zap.Logger) {
	if logger == nil {
		panic("nil logger")
	}
	c.logger = logger
}

// getFD retrieves an open file descriptor for the specified device path.
// It reuses descriptors when possible and evicts the least recently used entry
// when the cache reaches its capacity.
func (c *FDCache) getFD(devicePath string) (int, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if elem, ok := c.fds[devicePath]; ok {
		c.order.MoveToFront(elem)
		entry, ok := elem.Value.(*fdCacheEntry)
		if !ok {
			return -1, fmt.Errorf("invalid cache entry type")
		}
		return entry.fd, nil
	}

	fd, err := unix.Open(devicePath, unix.O_RDONLY|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, fmt.Errorf("failed to open device %s: %w", devicePath, err)
	}

	if c.order.Len() >= c.size {
		if back := c.order.Back(); back != nil {
			if entry, ok := back.Value.(*fdCacheEntry); ok {
				if err := unix.Close(entry.fd); err != nil {
					c.logger.Warn("failed to close fd", zap.String("path", entry.path), zap.Error(err))
				}
				delete(c.fds, entry.path)
			}
			c.order.Remove(back)
		}
	}

	elem := c.order.PushFront(&fdCacheEntry{path: devicePath, fd: fd})
	c.fds[devicePath] = elem
	return fd, nil
}

// Close releases all cached file descriptors and resets the cache state.
func (c *FDCache) Close() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	for _, elem := range c.fds {
		entry, ok := elem.Value.(*fdCacheEntry)
		if !ok {
			continue
		}
		if err := unix.Close(entry.fd); err != nil {
			c.logger.Warn("failed to close fd", zap.String("path", entry.path), zap.Error(err))
		}
	}
	c.fds = make(map[string]*list.Element, c.size)
	c.order.Init()
}
