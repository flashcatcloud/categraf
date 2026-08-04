package snmp

import (
	"log"
	"sync"
	"time"
)

const maxDependencyCacheEntries = 100000

const (
	dependencyCacheResultHit      = "hit"
	dependencyCacheResultMiss     = "miss"
	dependencyCacheResultExpired  = "expired"
	dependencyCacheResultDisabled = "disabled"
	dependencyCacheResultEvicted  = "evicted"
)

type dependencyCache struct {
	mu         sync.Mutex
	ttl        time.Duration
	closed     bool
	maxEntries int
	entries    int
	evictions  int64

	tableFields map[dependencyFieldKey]dependencySnapshot
	secondary   map[string]secondarySnapshot
	topTags     map[string]dependencyValue
}

type dependencyFieldKey struct {
	tableID   string
	fieldName string
}

type dependencySnapshot struct {
	values    map[string]interface{}
	expiresAt time.Time
}

type secondarySnapshot struct {
	values    map[string]string
	expiresAt time.Time
}

type dependencyValue struct {
	value     interface{}
	expiresAt time.Time
}

type dependencyCacheStats struct {
	entries   int
	evictions int64
}

func newDependencyCache(ttl time.Duration) *dependencyCache {
	return &dependencyCache{
		ttl:         ttl,
		maxEntries:  maxDependencyCacheEntries,
		tableFields: make(map[dependencyFieldKey]dependencySnapshot),
		secondary:   make(map[string]secondarySnapshot),
		topTags:     make(map[string]dependencyValue),
	}
}

func (c *dependencyCache) setMaxEntries(maxEntries int) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if maxEntries <= 0 {
		maxEntries = maxDependencyCacheEntries
	}
	c.maxEntries = maxEntries
	c.enforceCapacityLocked()
}

func (c *dependencyCache) setTTL(ttl time.Duration) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	c.ttl = ttl
	c.closed = false
	if ttl <= 0 {
		c.clearLocked()
	}
}

func (c *dependencyCache) disable() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	c.closed = true
	c.ttl = 0
	c.clearLocked()
}

func (c *dependencyCache) clearLocked() {
	c.tableFields = make(map[dependencyFieldKey]dependencySnapshot)
	c.secondary = make(map[string]secondarySnapshot)
	c.topTags = make(map[string]dependencyValue)
	c.entries = 0
}

func (c *dependencyCache) enabledLocked() bool {
	return c != nil && !c.closed && c.ttl > 0
}

func (c *dependencyCache) replaceTableField(tableID, fieldName string, values map[string]interface{}, now time.Time) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.enabledLocked() {
		return
	}

	c.pruneExpiredLocked(now)
	key := dependencyFieldKey{tableID: tableID, fieldName: fieldName}
	if old, ok := c.tableFields[key]; ok {
		c.entries -= len(old.values)
	}
	c.tableFields[key] = dependencySnapshot{
		values:    cloneInterfaceMap(values),
		expiresAt: now.Add(c.ttl),
	}
	c.entries += len(values)
	c.enforceCapacityLocked()
}

func (c *dependencyCache) tableField(tableID, fieldName string, now time.Time) (map[string]interface{}, bool, string) {
	if c == nil {
		return nil, false, dependencyCacheResultDisabled
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.enabledLocked() {
		return nil, false, dependencyCacheResultDisabled
	}

	key := dependencyFieldKey{tableID: tableID, fieldName: fieldName}
	snapshot, ok := c.tableFields[key]
	if !ok {
		return nil, false, dependencyCacheResultMiss
	}
	if now.After(snapshot.expiresAt) {
		delete(c.tableFields, key)
		c.entries -= len(snapshot.values)
		return nil, false, dependencyCacheResultExpired
	}
	return cloneInterfaceMap(snapshot.values), true, dependencyCacheResultHit
}

func (c *dependencyCache) replaceSecondary(tableID string, values map[string]string, now time.Time) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.enabledLocked() {
		return
	}

	c.pruneExpiredLocked(now)
	if old, ok := c.secondary[tableID]; ok {
		c.entries -= len(old.values)
	}
	c.secondary[tableID] = secondarySnapshot{
		values:    cloneStringMap(values),
		expiresAt: now.Add(c.ttl),
	}
	c.entries += len(values)
	c.enforceCapacityLocked()
}

func (c *dependencyCache) secondaryMapping(tableID string, now time.Time) (map[string]string, bool, string) {
	if c == nil {
		return nil, false, dependencyCacheResultDisabled
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.enabledLocked() {
		return nil, false, dependencyCacheResultDisabled
	}

	snapshot, ok := c.secondary[tableID]
	if !ok {
		return nil, false, dependencyCacheResultMiss
	}
	if now.After(snapshot.expiresAt) {
		delete(c.secondary, tableID)
		c.entries -= len(snapshot.values)
		return nil, false, dependencyCacheResultExpired
	}
	return cloneStringMap(snapshot.values), true, dependencyCacheResultHit
}

func (c *dependencyCache) storeTopTag(tagName string, value interface{}, now time.Time) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.enabledLocked() {
		return
	}

	c.pruneExpiredLocked(now)
	if _, ok := c.topTags[tagName]; !ok {
		c.entries++
	}
	c.topTags[tagName] = dependencyValue{
		value:     value,
		expiresAt: now.Add(c.ttl),
	}
	c.enforceCapacityLocked()
}

func (c *dependencyCache) deleteTopTag(tagName string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.topTags[tagName]; ok {
		delete(c.topTags, tagName)
		c.entries--
	}
}

func (c *dependencyCache) topTag(tagName string, now time.Time) (interface{}, bool, string) {
	if c == nil {
		return nil, false, dependencyCacheResultDisabled
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.enabledLocked() {
		return nil, false, dependencyCacheResultDisabled
	}

	value, ok := c.topTags[tagName]
	if !ok {
		return nil, false, dependencyCacheResultMiss
	}
	if now.After(value.expiresAt) {
		delete(c.topTags, tagName)
		c.entries--
		return nil, false, dependencyCacheResultExpired
	}
	return value.value, true, dependencyCacheResultHit
}

func (c *dependencyCache) stats() dependencyCacheStats {
	if c == nil {
		return dependencyCacheStats{}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return dependencyCacheStats{
		entries:   c.entryCountLocked(),
		evictions: c.evictions,
	}
}

func (c *dependencyCache) pruneExpiredLocked(now time.Time) {
	for key, snapshot := range c.tableFields {
		if now.After(snapshot.expiresAt) {
			delete(c.tableFields, key)
			c.entries -= len(snapshot.values)
		}
	}
	for key, snapshot := range c.secondary {
		if now.After(snapshot.expiresAt) {
			delete(c.secondary, key)
			c.entries -= len(snapshot.values)
		}
	}
	for key, value := range c.topTags {
		if now.After(value.expiresAt) {
			delete(c.topTags, key)
			c.entries--
		}
	}
}

func (c *dependencyCache) enforceCapacityLocked() {
	maxEntries := c.maxEntries
	if maxEntries <= 0 {
		maxEntries = maxDependencyCacheEntries
	}
	if c.entryCountLocked() <= maxEntries {
		return
	}
	c.pruneExpiredLocked(time.Now())
	if c.entryCountLocked() <= maxEntries {
		return
	}
	c.evictions++
	log.Printf("W! snmp dependency cache exceeded %d entries, clearing cache", maxEntries)
	c.clearLocked()
}

func (c *dependencyCache) entryCountLocked() int {
	if c.entries < 0 {
		c.entries = 0
	}
	return c.entries
}

func cloneInterfaceMap(in map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneStringMap(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
