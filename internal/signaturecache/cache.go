package signaturecache

import (
	"container/list"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Cache persists volume signatures on disk and keeps an in-memory LRU index.
// Entries are stored under basePath/<vg>/<lv>.sig.
// Entries expire after ttl and the cache evicts least recently used entries
// when max entries is exceeded.
type Cache struct {
	basePath string
	ttl      time.Duration
	max      int

	mu      sync.Mutex
	entries map[string]*list.Element
	lru     *list.List // *entry, front = most recent
}

type entry struct {
	key     string
	vg      string
	lv      string
	digest  []byte
	size    int64
	expires time.Time
}

// fileEntry is persisted to disk as JSON.
type fileEntry struct {
	Size      int64     `json:"size"`
	DigestHex string    `json:"digest"`
	Timestamp time.Time `json:"ts"`
}

// New returns a cache storing entries under basePath with the provided ttl and max.
func New(basePath string, ttl time.Duration, max int) *Cache {
	return &Cache{
		basePath: basePath,
		ttl:      ttl,
		max:      max,
		entries:  make(map[string]*list.Element),
		lru:      list.New(),
	}
}

func (c *Cache) path(vg, lv string) string {
	return filepath.Join(c.basePath, vg, lv+".sig")
}

// Get returns the cached digest for vg/lv if present, unexpired, and size matches.
// It loads the entry from disk on cache miss. Returns false if not found or invalid.
func (c *Cache) Get(vg, lv string, size int64) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := vg + "/" + lv
	if el, ok := c.entries[key]; ok {
		e := el.Value.(*entry)
		if c.expired(e) || e.size != size {
			c.removeElement(el)
			os.Remove(c.path(vg, lv))
			return nil, false
		}
		e.expires = time.Now().Add(c.ttl)
		c.lru.MoveToFront(el)
		return e.digest, true
	}
	fe, err := c.loadFile(vg, lv)
	if err != nil {
		return nil, false
	}
	if fe.Size != size || c.isExpired(fe.Timestamp) {
		os.Remove(c.path(vg, lv))
		return nil, false
	}
	digest, err := hex.DecodeString(fe.DigestHex)
	if err != nil {
		os.Remove(c.path(vg, lv))
		return nil, false
	}
	e := &entry{key: key, vg: vg, lv: lv, digest: digest, size: fe.Size, expires: time.Now().Add(c.ttl)}
	c.entries[key] = c.lru.PushFront(e)
	c.evict()
	return digest, true
}

// Put stores the digest for vg/lv and writes it to disk.
func (c *Cache) Put(vg, lv string, size int64, digest []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := vg + "/" + lv
	if el, ok := c.entries[key]; ok {
		c.lru.Remove(el)
	}
	e := &entry{key: key, vg: vg, lv: lv, digest: digest, size: size, expires: time.Now().Add(c.ttl)}
	c.entries[key] = c.lru.PushFront(e)
	if err := c.writeFile(vg, lv, size, digest); err != nil {
		c.removeElement(c.entries[key])
		return err
	}
	c.evict()
	return nil
}

// Check verifies that the cache entry for vg/lv matches size and digest.
// On mismatch or expiration the entry is removed and false is returned.
func (c *Cache) Check(vg, lv string, size int64, digest []byte) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := vg + "/" + lv
	if el, ok := c.entries[key]; ok {
		e := el.Value.(*entry)
		if c.expired(e) || e.size != size || !equal(e.digest, digest) {
			c.removeElement(el)
			os.Remove(c.path(vg, lv))
			return false
		}
		e.expires = time.Now().Add(c.ttl)
		c.lru.MoveToFront(el)
		return true
	}
	fe, err := c.loadFile(vg, lv)
	if err != nil {
		return false
	}
	if fe.Size != size || c.isExpired(fe.Timestamp) {
		os.Remove(c.path(vg, lv))
		return false
	}
	d, err := hex.DecodeString(fe.DigestHex)
	if err != nil || !equal(d, digest) {
		os.Remove(c.path(vg, lv))
		return false
	}
	e := &entry{key: key, vg: vg, lv: lv, digest: d, size: fe.Size, expires: time.Now().Add(c.ttl)}
	c.entries[key] = c.lru.PushFront(e)
	c.evict()
	return true
}

func (c *Cache) evict() {
	for c.max > 0 && c.lru.Len() > c.max {
		el := c.lru.Back()
		if el == nil {
			return
		}
		c.removeElement(el)
	}
}

func (c *Cache) removeElement(el *list.Element) {
	c.lru.Remove(el)
	e := el.Value.(*entry)
	delete(c.entries, e.key)
	os.Remove(c.path(e.vg, e.lv))
}

func (c *Cache) expired(e *entry) bool {
	return c.ttl > 0 && time.Now().After(e.expires)
}

func (c *Cache) isExpired(ts time.Time) bool {
	return c.ttl > 0 && time.Since(ts) > c.ttl
}

func (c *Cache) loadFile(vg, lv string) (*fileEntry, error) {
	data, err := os.ReadFile(c.path(vg, lv))
	if err != nil {
		return nil, err
	}
	var fe fileEntry
	if err := json.Unmarshal(data, &fe); err != nil {
		return nil, err
	}
	if fe.Timestamp.IsZero() {
		return nil, errors.New("invalid timestamp")
	}
	return &fe, nil
}

func (c *Cache) writeFile(vg, lv string, size int64, digest []byte) error {
	fe := fileEntry{Size: size, DigestHex: hex.EncodeToString(digest), Timestamp: time.Now()}
	data, err := json.Marshal(fe)
	if err != nil {
		return err
	}
	path := c.path(vg, lv)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func equal(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
