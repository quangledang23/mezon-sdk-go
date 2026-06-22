package mezon

import "sync"

// CacheManager is a thread-safe, insertion-ordered cache with a fetcher used to
// lazily load missing entries, port of CacheManager + Collection in
// src/mezon-client/utils. maxSize <= 0 means unbounded.
type CacheManager[K comparable, V any] struct {
	mu      sync.RWMutex
	items   map[K]V
	order   []K
	maxSize int
	fetcher func(K) (V, error)
}

// NewCacheManager creates a cache. fetcher may be nil (Fetch then only returns
// cached values).
func NewCacheManager[K comparable, V any](fetcher func(K) (V, error), maxSize int) *CacheManager[K, V] {
	return &CacheManager[K, V]{
		items:   make(map[K]V),
		maxSize: maxSize,
		fetcher: fetcher,
	}
}

// Get returns the cached value and whether it was present.
func (c *CacheManager[K, V]) Get(id K) (V, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	v, ok := c.items[id]
	return v, ok
}

// Set stores value under id, evicting the oldest entry when maxSize is reached.
func (c *CacheManager[K, V]) Set(id K, value V) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.items[id]; !exists {
		if c.maxSize > 0 && len(c.items) >= c.maxSize && len(c.order) > 0 {
			oldest := c.order[0]
			c.order = c.order[1:]
			delete(c.items, oldest)
		}
		c.order = append(c.order, id)
	}
	c.items[id] = value
}

// Fetch returns the cached value, loading it via the fetcher on a miss.
func (c *CacheManager[K, V]) Fetch(id K) (V, error) {
	if v, ok := c.Get(id); ok {
		return v, nil
	}
	var zero V
	if c.fetcher == nil {
		return zero, ErrNotFound
	}
	v, err := c.fetcher(id)
	if err != nil {
		return zero, err
	}
	c.Set(id, v)
	return v, nil
}

// Delete removes id from the cache.
func (c *CacheManager[K, V]) Delete(id K) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.items[id]; !ok {
		return false
	}
	delete(c.items, id)
	for i, k := range c.order {
		if k == id {
			c.order = append(c.order[:i], c.order[i+1:]...)
			break
		}
	}
	return true
}

// Values returns a snapshot of cached values in insertion order.
func (c *CacheManager[K, V]) Values() []V {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]V, 0, len(c.order))
	for _, k := range c.order {
		out = append(out, c.items[k])
	}
	return out
}

// Size returns the number of cached entries.
func (c *CacheManager[K, V]) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.items)
}
