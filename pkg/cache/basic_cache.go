package cache

import (
	"sync"
	"time"
)

// BasicCache is a simple threadsafe cache
type BasicCache[R comparable] struct {
	m        sync.RWMutex
	cache    map[R]interface{}
	modified int64
}

// NewBasicCache Creates new BasicCache
func NewBasicCache[R comparable]() *BasicCache[R] {
	return &BasicCache[R]{
		cache: make(map[R]interface{}),
	}
}

// Add adds value to cache for specified key
// It will overwrite any existing value
func (b *BasicCache[R]) Add(k R, v interface{}) {
	b.m.Lock()
	defer b.m.Unlock()

	b.cache[k] = v
	b.modified = time.Now().Unix()
}

// Get gets interface for specified key and a boolean that's false when the key is not found
func (b *BasicCache[R]) Get(k R) (interface{}, bool) {
	b.m.RLock()
	defer b.m.RUnlock()

	v, found := b.cache[k]

	return v, found
}

// Remove removes an entry from the cache if it exists
func (b *BasicCache[R]) Remove(k R) {
	b.m.Lock()
	defer b.m.Unlock()

	delete(b.cache, k)
	b.modified = time.Now().Unix()
}

// Size returns the current size of the cache
func (b *BasicCache[R]) Size() int {
	b.m.Lock()
	defer b.m.Unlock()

	return len(b.cache)
}

// GetModified gets interface for specified key or error
func (b *BasicCache[R]) GetModified() int64 {
	b.m.RLock()
	defer b.m.RUnlock()

	return b.modified
}

// Items returns a map with the elements in the cache
func (b *BasicCache[R]) Items() map[R]interface{} {
	items := map[R]interface{}{}

	b.m.RLock()
	defer b.m.RUnlock()
	for k, v := range b.cache {
		items[k] = v
	}

	return items
}
